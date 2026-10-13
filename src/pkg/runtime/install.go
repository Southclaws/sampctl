package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

type runtimeInstallHooks struct {
	after func(string) error
}

func (hooks runtimeInstallHooks) checkpoint(point string) error {
	if hooks.after == nil {
		return nil
	}
	return hooks.after(point)
}

func installRuntime(ctx context.Context, root, source string, current runtimeManifest, previous *runtimeManifest, hooks runtimeInstallHooks) (err error) {
	journal, err := prepareRuntimeJournal(ctx, root, source, current, previous)
	if err != nil {
		_, statErr := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename))
		return &RuntimeInstallError{
			Runtime:         current.Version,
			Restored:        previous != nil && verifyRuntimeManifest(*previous, root) == nil,
			RecoveryPending: !errors.Is(statErr, os.ErrNotExist),
			Err:             err,
		}
	}
	defer func() {
		if err == nil {
			return
		}
		failure := &RuntimeInstallError{Runtime: current.Version, Committed: journal.Phase == "committed", RecoveryPending: true, Err: err}
		if !failure.Committed {
			recoveryErr := recoverRuntimeLocked(context.WithoutCancel(ctx), root)
			failure.Err = errors.Join(failure.Err, recoveryErr)
			if recoveryErr == nil && previous != nil {
				failure.Restored = verifyRuntimeManifest(*previous, root) == nil
			}
		}
		_, pendingErr := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename))
		failure.RecoveryPending = !errors.Is(pendingErr, os.ErrNotExist)
		err = failure
	}()
	if err := hooks.checkpoint("prepared"); err != nil {
		return err
	}
	for index, entry := range journal.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := hooks.checkpoint("before:" + entry.Path); err != nil {
			return err
		}
		before, err := runtimeFileMatches(root, entry.Path, runtimeBeforePlatform(journal, entry), entry.Before)
		if err != nil || !before {
			return errors.Join(fmt.Errorf("runtime file %s changed before installation: %w", entry.Path, os.ErrInvalid), err)
		}
		target, err := runtimeFilePath(root, entry.Path)
		if err != nil {
			return err
		}
		if entry.After == nil {
			if err := os.Remove(target); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			match, err := runtimeFileMatches(filepath.Join(root, journal.Stage), fmt.Sprintf("new-%06d", index), current.Platform, entry.After)
			if err != nil || !match {
				return errors.Join(fmt.Errorf("staged runtime file %s changed: %w", entry.Path, os.ErrInvalid), err)
			}
			if err := os.Rename(runtimeStageFile(root, journal, index, "new"), target); err != nil {
				return err
			}
		}
		if err := errors.Join(syncRuntimeParents(root, filepath.Dir(target)), fs.SyncDir(filepath.Join(root, journal.Stage))); err != nil {
			return err
		}
		if err := hooks.checkpoint("published:" + entry.Path); err != nil {
			return err
		}
	}
	if err := verifyRuntimeManifest(current, root); err != nil {
		return err
	}
	journal.Phase = "committed"
	if err := validateRuntimeRecoveryState(root, journal); err != nil {
		journal.Phase = "pending"
		return err
	}
	if err := writeRuntimeJournal(root, journal); err != nil {
		// The atomic write may have published the commit before directory sync failed.
		persisted, readErr := readRuntimeJournal(root)
		if readErr != nil || persisted.Phase != "committed" {
			journal.Phase = "pending"
		}
		return errors.Join(err, readErr)
	}
	if err := hooks.checkpoint("committed"); err != nil {
		return err
	}
	return cleanupRuntimeJournalWithHooks(ctx, root, journal, hooks)
}

func prepareRuntimeJournal(ctx context.Context, root, source string, current runtimeManifest, previous *runtimeManifest) (_ *runtimeJournal, err error) {
	if err := validateInstalledRuntimeManifest(current, root); err != nil {
		return nil, err
	}
	if err := validateRuntimePackage(current, source); err != nil {
		return nil, err
	}
	tracked := make(map[string]runtimeFileInfo)
	if previous != nil {
		if err := validateInstalledRuntimeManifest(*previous, root); err != nil {
			return nil, err
		}
		for _, file := range previous.Files {
			tracked[strings.ToLower(file.Path)] = file
		}
	}
	journal := &runtimeJournal{Format: 1, Phase: "pending", Previous: previous, Current: current}
	for _, file := range current.Files {
		entry := runtimeJournalEntry{Path: file.Path, After: &file}
		path, err := runtimeFilePath(root, file.Path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Lstat(path); err == nil {
			before, owned := tracked[strings.ToLower(file.Path)]
			if owned {
				match, err := runtimeFileMatches(root, file.Path, previous.Platform, &before)
				if err != nil || !match {
					return nil, errors.Join(fmt.Errorf("runtime file %s was modified. Move it aside before updating: %w", file.Path, os.ErrExist), err)
				}
			} else {
				before = file
				match, err := runtimeFileMatches(root, file.Path, current.Platform, &before)
				if err != nil || !match {
					return nil, errors.Join(fmt.Errorf("runtime file %s already exists and is not tracked by sampctl: %w", file.Path, os.ErrExist), err)
				}
			}
			entry.Before = &before
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		journal.Files = append(journal.Files, entry)
	}
	if previous != nil {
		obsolete, err := obsoleteRuntimeFiles(*previous, current, root)
		if err != nil {
			return nil, err
		}
		for _, file := range obsolete.Files {
			journal.Files = append(journal.Files, runtimeJournalEntry{Path: file.Path, Before: &file})
		}
	}
	ownershipPath, err := runtimeFilePath(root, installedRuntimeManifestFileName)
	if err != nil {
		return nil, err
	}
	ownership := runtimeJournalEntry{Path: installedRuntimeManifestFileName}
	if info, err := os.Lstat(ownershipPath); err == nil {
		hash, size, err := hashFile(ownershipPath)
		if err != nil {
			return nil, err
		}
		ownership.Before = &runtimeFileInfo{Path: installedRuntimeManifestFileName, Size: size, Hash: hash, Mode: uint32(info.Mode().Perm())}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	journal.Files = append(journal.Files, ownership)
	stage, err := os.MkdirTemp(root, runtimeOperationPrefix)
	if err != nil {
		return nil, err
	}
	journal.Stage = filepath.Base(stage)
	persisted := false
	defer func() {
		if !persisted {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	for index, entry := range journal.Files {
		if entry.Before != nil {
			if err := copyFileWithModeContext(ctx, filepath.Join(root, filepath.FromSlash(entry.Path)), runtimeStageFile(root, journal, index, "old"), os.FileMode(entry.Before.Mode)); err != nil {
				return nil, err
			}
		}
		if entry.After != nil {
			if err := copyFileWithModeContext(ctx, filepath.Join(source, filepath.FromSlash(entry.Path)), runtimeStageFile(root, journal, index, "new"), os.FileMode(entry.After.Mode)); err != nil {
				return nil, err
			}
		}
	}
	index := len(journal.Files) - 1
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, err
	}
	path := runtimeStageFile(root, journal, index, "new")
	if err := fs.WriteFileAtomic(path, data, 0o700, 0o600); err != nil {
		return nil, err
	}
	hash, size, err := hashFile(path)
	if err != nil {
		return nil, err
	}
	journal.Files[index].After = &runtimeFileInfo{Path: installedRuntimeManifestFileName, Hash: hash, Size: size, Mode: 0o600}
	if err := validateRuntimeJournal(root, journal); err != nil {
		return nil, err
	}
	if err := validateRuntimeRecoveryState(root, journal); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := writeRuntimeJournal(root, journal); err != nil {
		// Preserve backups whenever publication may have succeeded.
		_, statErr := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename))
		persisted = !errors.Is(statErr, os.ErrNotExist)
		return nil, err
	}
	persisted = true
	return journal, nil
}
