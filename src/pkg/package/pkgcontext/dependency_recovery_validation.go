package pkgcontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func regularRecoveryFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("recovery file %s must be a regular file", path)
	}

	return nil
}

func validateRecoveryCleanup(ctx context.Context, root string, expected []dependencyRecoveryCleanupEntry) error {
	entries, err := recoveryCleanupEntries(ctx, root)
	if err != nil {
		return err
	}

	recorded := make(map[string]dependencyRecoveryCleanupEntry, len(expected))
	for _, entry := range expected {
		recorded[entry.Path] = entry
	}

	for _, entry := range entries {
		if original, exists := recorded[entry.Path]; !exists || original != entry {
			return fmt.Errorf("staged path %s is unrecognized or changed. Left in place for manual recovery", filepath.Join(root, filepath.FromSlash(entry.Path)))
		}
	}

	return nil
}

// This only inspects ancestors. Recovery must never create a path while deciding
// whether recorded state is safe to restore.
func (journal *dependencyJournal) safePath(relative string) error {
	current := journal.project
	for _, component := range append([]string{"."}, strings.Split(relative, "/")...) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("recovery path contains a symbolic link: %s", current)
		}
		if current != journal.path(relative) && !info.IsDir() {
			return fmt.Errorf("recovery ancestor is not a directory: %s", current)
		}
	}

	return nil
}

func recoveryFileEqual(path string, left, right []byte) bool {
	if bytes.Equal(left, right) {
		return true
	}

	if path != lockfile.Filename {
		return false
	}

	var l, r map[string]json.RawMessage
	if json.Unmarshal(left, &l) != nil || json.Unmarshal(right, &r) != nil {
		return false
	}

	delete(l, "generated")
	delete(r, "generated")
	lb, le := json.Marshal(l)
	rb, re := json.Marshal(r)
	// Decode again so whitespace inside nested JSON values is insignificant.
	var lv, rv any
	if le != nil || re != nil || json.Unmarshal(lb, &lv) != nil || json.Unmarshal(rb, &rv) != nil {
		return false
	}

	lb, le = json.Marshal(lv)
	rb, re = json.Marshal(rv)
	if le != nil || re != nil {
		return false
	}

	return bytes.Equal(lb, rb)
}

func (journal *dependencyJournal) validateState(ctx context.Context) error {
	if err := journal.validate(); err != nil {
		return err
	}

	for _, file := range journal.Files {
		if err := journal.safePath(file.Path); err != nil {
			return err
		}
		if err := regularRecoveryFile(journal.path(file.Path)); err != nil {
			return err
		}
		data, err := os.ReadFile(journal.path(file.Path))
		if errors.Is(err, os.ErrNotExist) {
			// Missing files are only expected while an active operation is being restored.
			if (journal.Phase == "committed" && (file.Exists || len(file.Writes) != 0)) || (journal.Phase == "restored" && file.Exists) {
				return fmt.Errorf("%s project file is missing: %s", journal.Phase, journal.path(file.Path))
			}
			continue
		}
		if err != nil {
			return err
		}
		known := file.Exists && bytes.Equal(file.Data, data)
		if journal.Phase == "committed" && len(file.Writes) != 0 {
			known = recoveryFileEqual(file.Path, file.Writes[len(file.Writes)-1], data)
		} else if journal.Phase != "restored" {
			for _, write := range file.Writes {
				known = known || recoveryFileEqual(file.Path, write, data)
			}
		}
		if !known {
			return fmt.Errorf("project file %s does not match the recovery record. Keep the record and inspect the file", journal.path(file.Path))
		}
	}

	for _, stage := range journal.Stages {
		if err := journal.safePath(stage.Path); err != nil {
			return err
		}

		entries, err := os.ReadDir(journal.path(stage.Path))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}

		if stage.Cleanup {
			if err := validateRecoveryCleanup(ctx, journal.path(stage.Path), stage.CleanupEntries); err != nil {
				return err
			}
		}

		markerPath := filepath.Join(journal.path(stage.Path), ".sampctl-owner")
		if err := regularRecoveryFile(markerPath); err != nil {
			return err
		}

		marker, err := os.ReadFile(markerPath)
		if !stage.Cleanup && (err != nil || string(marker) != journal.ID) {
			if len(entries) == 0 && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("staging directory ownership cannot be verified: %s", journal.path(stage.Path))
		}

		for _, entry := range entries {
			if entry.Name() == ".sampctl-owner" {
				if !entry.Type().IsRegular() {
					return fmt.Errorf("invalid staging ownership marker: %s", journal.path(stage.Path))
				}
				continue
			}
			allowed := slices.Contains(stage.Allowed, entry.Name())
			for _, name := range stage.Allowed {
				if !strings.HasPrefix(name, ".") && (strings.HasPrefix(entry.Name(), "."+name+".clone-") || strings.HasPrefix(entry.Name(), "."+name+".update-")) {
					allowed = true
				}
			}
			if !allowed || entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("unexpected staged path %s. Left in place for manual recovery", filepath.Join(journal.path(stage.Path), entry.Name()))
			}
		}
	}

	for _, change := range journal.Changes {
		cleaning := false
		for _, stage := range journal.Stages {
			if recoveryWithin(stage.Path, change.Source) {
				cleaning = stage.Cleanup
			}
		}

		for _, path := range []string{change.Target, change.Source, change.Backup} {
			if path != "" {
				if err := journal.safePath(path); err != nil {
					return err
				}
			}
		}

		target, err := recoveryTreeHash(ctx, journal.path(change.Target), false)
		if err != nil {
			return err
		}

		var source, backup string
		backupExists := false
		// Cleanup inventories already verify staged contents. Installed
		// destinations always require the fresh tree fingerprint above.
		if !cleaning {
			source, err = recoveryTreeHash(ctx, journal.path(change.Source), false)
			if err != nil {
				return err
			}
			if change.Backup != "" {
				backup, err = recoveryTreeHash(ctx, journal.path(change.Backup), false)
				if err != nil {
					return err
				}
				backupExists = backup != ""
			}
		} else if journal.Phase == "restored" && change.Backup != "" {
			// A restored installation must still have no remaining backup.
			_, err := os.Lstat(journal.path(change.Backup))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			backupExists = err == nil
		}

		if source != "" && source != change.NewHash && !cleaning {
			return fmt.Errorf("staged replacement has changed: %s", journal.path(change.Source))
		}

		if journal.Phase == "committed" {
			if target != change.NewHash || (source != "" && !cleaning) || (!cleaning && backup != "" && backup != change.OldHash) {
				return fmt.Errorf("committed dependency %s is missing or changed. Backup kept at %s", journal.path(change.Target), journal.path(change.Backup))
			}
			continue
		}

		if journal.Phase == "restored" {
			if target != change.OldHash || backupExists {
				return fmt.Errorf("restored dependency %s has changed. Keep the recovery record and inspect the directory", journal.path(change.Target))
			}
			continue
		}

		if backup != "" {
			if backup != change.OldHash || (target != "" && target != change.NewHash) || (target != "" && source != "") {
				return fmt.Errorf("cannot restore dependency %s from backup %s automatically. Inspect both directories", journal.path(change.Target), journal.path(change.Backup))
			}
		} else if change.OldHash != "" && target != change.OldHash {
			return fmt.Errorf("previous dependency %s cannot be restored: recorded backup %s is missing", journal.path(change.Target), journal.path(change.Backup))
		} else if change.OldHash == "" && target != "" && (target != change.NewHash || source != "") {
			return fmt.Errorf("cannot remove unexpected dependency %s during recovery. Inspect the directory", journal.path(change.Target))
		}
	}

	return ctx.Err()
}
