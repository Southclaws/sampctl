package pkgcontext

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

// DependencyRecoveryFilename contains recovery state for the project dependency operation.
const DependencyRecoveryFilename = ".sampctl-dependencies.json"

type dependencyRecoveryFile struct {
	Path   string   `json:"path"`
	Data   []byte   `json:"data"`
	Mode   uint32   `json:"mode"`
	Exists bool     `json:"exists"`
	Hash   string   `json:"hash"`
	Writes [][]byte `json:"writes,omitempty"`
}

type dependencyRecoveryStage struct {
	Path           string                           `json:"path"`
	Allowed        []string                         `json:"allowed,omitempty"`
	Cleanup        bool                             `json:"cleanup,omitempty"`
	CleanupEntries []dependencyRecoveryCleanupEntry `json:"cleanup_entries,omitempty"`
}

type dependencyRecoveryCleanupEntry struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
	Hash string `json:"hash,omitempty"`
	Link string `json:"link,omitempty"`
}

type dependencyRecoveryChange struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Backup  string `json:"backup,omitempty"`
	OldHash string `json:"old_hash,omitempty"`
	NewHash string `json:"new_hash"`
	Phase   string `json:"phase"`
}

// Intent is persisted before each rename. Recovery uses fingerprints and actual
// paths because a process can exit between a rename and its phase update.
type dependencyJournal struct {
	Version   int                        `json:"version"`
	ID        string                     `json:"id"`
	Operation string                     `json:"operation"`
	Phase     string                     `json:"phase"`
	Files     []dependencyRecoveryFile   `json:"files,omitempty"`
	Stages    []dependencyRecoveryStage  `json:"stages,omitempty"`
	Changes   []dependencyRecoveryChange `json:"changes,omitempty"`
	project   string
}

func (journal *dependencyJournal) path(relative string) string {
	return filepath.Join(journal.project, filepath.FromSlash(relative))
}

func (journal *dependencyJournal) relative(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	relative, err := filepath.Rel(journal.project, path)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return "", fmt.Errorf("recovery path %s is outside project %s", path, journal.project)
	}

	return filepath.ToSlash(relative), nil
}

func (journal *dependencyJournal) save() error {
	path := journal.path(DependencyRecoveryFilename)
	if err := regularRecoveryFile(path); err != nil {
		return err
	}

	if err := fs.WriteJSONAtomic(path, journal, 0o700, 0o600); err != nil {
		return fmt.Errorf("persist dependency recovery record %s: %w", path, err)
	}

	return nil
}

func newDependencyJournal(project, operation string, files ...projectFileSnapshot) (*dependencyJournal, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}

	journal := &dependencyJournal{Version: 1, ID: rand.Text(), Operation: operation, Phase: "active", project: project}
	for _, file := range files {
		if file.path == "" {
			continue
		}
		relative, err := journal.relative(file.path)
		if err != nil {
			return nil, err
		}
		recorded := dependencyRecoveryFile{Path: relative, Data: file.data, Mode: uint32(file.mode.Perm()), Exists: file.exists}
		recorded.Hash = recoverySnapshotHash(recorded)
		journal.Files = append(journal.Files, recorded)
	}

	if err := journal.save(); err != nil {
		return nil, err
	}

	return journal, nil
}

func (journal *dependencyJournal) recordWrite(path string, data []byte) error {
	relative, err := journal.relative(path)
	if err != nil {
		return err
	}

	for i := range journal.Files {
		if journal.Files[i].Path == relative {
			journal.Files[i].Writes = append(journal.Files[i].Writes, slices.Clone(data))
			return journal.save()
		}
	}

	return fmt.Errorf("project file %s has no recovery snapshot", path)
}

func readDependencyJournal(project string) (*dependencyJournal, error) {
	project, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(project, DependencyRecoveryFilename)
	if err := regularRecoveryFile(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	journal := &dependencyJournal{project: project}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(journal); err != nil {
		return nil, fmt.Errorf("invalid dependency recovery record %s: %w", path, err)
	}

	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("invalid trailing data in dependency recovery record %s", path)
	}

	if err := journal.validate(); err != nil {
		return nil, fmt.Errorf("invalid dependency recovery record %s: %w", path, err)
	}

	return journal, nil
}

func safeRecoveryRelative(path string) bool {
	return path != "" && path != "." && !strings.ContainsAny(path, "\\:") && filepath.IsLocal(filepath.FromSlash(path)) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) == path
}

func recoveryWithin(root, path string) bool { return strings.HasPrefix(path, root+"/") }

func (journal *dependencyJournal) validate() error {
	if journal.Version != 1 || journal.ID == "" || strings.ContainsAny(journal.ID, "/\\:") || journal.Operation == "" {
		return errors.New("unsupported version or missing operation identity")
	}

	if journal.Phase != "active" && journal.Phase != "committed" && journal.Phase != "restored" {
		return fmt.Errorf("unknown operation phase %q", journal.Phase)
	}

	seen := make(map[string]bool)
	for _, file := range journal.Files {
		if file.Path != "pawn.json" && file.Path != "pawn.yaml" && file.Path != lockfile.Filename {
			return fmt.Errorf("invalid project snapshot path %q", file.Path)
		}
		if seen[file.Path] || file.Mode > 0o777 || (!file.Exists && len(file.Data) != 0) || file.Hash != recoverySnapshotHash(file) {
			return fmt.Errorf("invalid project snapshot %q", file.Path)
		}
		seen[file.Path] = true
	}

	seen = make(map[string]bool)
	for _, stage := range journal.Stages {
		if !safeRecoveryRelative(stage.Path) || filepath.ToSlash(filepath.Dir(stage.Path)) != "dependencies" || !strings.HasPrefix(filepath.Base(stage.Path), ".sampctl-stage-"+journal.ID+"-") || seen[stage.Path] {
			return fmt.Errorf("invalid staging path %q", stage.Path)
		}
		seen[stage.Path] = true
		if stage.Cleanup && journal.Phase == "active" {
			return errors.New("uncommitted staging directory has cleanup intent")
		}
		if !stage.Cleanup && len(stage.CleanupEntries) != 0 {
			return errors.New("staging cleanup inventory has no cleanup intent")
		}

		cleanupPaths := make(map[string]bool, len(stage.CleanupEntries))
		for _, entry := range stage.CleanupEntries {
			mode := os.FileMode(entry.Mode)
			key := strings.ToLower(entry.Path)
			if !safeRecoveryRelative(entry.Path) || cleanupPaths[key] || mode&^(os.ModePerm|os.ModeType|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
				return fmt.Errorf("invalid cleanup inventory path or mode %q", entry.Path)
			}
			cleanupPaths[key] = true
			switch {
			case mode.IsRegular():
				if !validRecoveryHash(entry.Hash) || entry.Link != "" {
					return fmt.Errorf("invalid cleanup file fingerprint %q", entry.Path)
				}
			case mode&os.ModeType == os.ModeDir:
				if entry.Hash != "" || entry.Link != "" {
					return fmt.Errorf("invalid cleanup directory %q", entry.Path)
				}
			case mode&os.ModeType == os.ModeSymlink:
				if entry.Hash != "" || entry.Link == "" {
					return fmt.Errorf("invalid cleanup symbolic link %q", entry.Path)
				}
			default:
				return fmt.Errorf("unsupported cleanup inventory type %q", entry.Path)
			}
		}

		for _, name := range stage.Allowed {
			if !safeRecoveryRelative(name) || strings.Contains(name, "/") || name == ".sampctl-owner" {
				return fmt.Errorf("invalid staged entry %q", name)
			}
		}
	}

	targets := make(map[string]bool)
	stagedPaths := make(map[string]bool)
	for _, change := range journal.Changes {
		if !safeRecoveryRelative(change.Source) || !safeRecoveryRelative(change.Target) || !recoveryWithin("dependencies", change.Target) {
			return fmt.Errorf("invalid installation paths for %q", change.Target)
		}

		name := strings.ToLower(strings.Split(change.Target, "/")[1])
		if strings.HasPrefix(name, ".sampctl-") || strings.EqualFold(change.Target, "dependencies/.resources") {
			return fmt.Errorf("reserved installation target %q", change.Target)
		}

		hasPrevious := change.OldHash != ""
		hasBackup := change.Backup != ""
		if !validRecoveryHash(change.NewHash) || (hasPrevious && !validRecoveryHash(change.OldHash)) || hasPrevious != hasBackup {
			return fmt.Errorf("invalid installation paths or fingerprints for %q", change.Target)
		}

		if change.Phase != "prepared" && change.Phase != "backed_up" && change.Phase != "promoted" {
			return fmt.Errorf("invalid installation phase for %q", change.Target)
		}

		targetKey := strings.ToLower(change.Target)
		for target := range targets {
			if target == targetKey || recoveryWithin(target, targetKey) || recoveryWithin(targetKey, target) {
				return fmt.Errorf("overlapping installation target %q", change.Target)
			}
		}
		targets[targetKey] = true

		for _, path := range []string{change.Source, change.Backup} {
			if path == "" {
				continue
			}
			key := strings.ToLower(path)
			for other := range stagedPaths {
				if key == other || recoveryWithin(key, other) || recoveryWithin(other, key) {
					return fmt.Errorf("overlapping staged path %q", path)
				}
			}
			stagedPaths[key] = true
		}

		owned := false
		for _, stage := range journal.Stages {
			if !recoveryWithin(stage.Path, change.Source) {
				continue
			}
			if change.Backup == "" {
				owned = true
				continue
			}
			if safeRecoveryRelative(change.Backup) && recoveryWithin(stage.Path, change.Backup) && filepath.ToSlash(filepath.Dir(change.Backup)) == stage.Path && strings.HasPrefix(filepath.Base(change.Backup), ".previous-") {
				owned = true
			}
		}
		if !owned || change.Backup == change.Source || recoveryWithin(change.Source, change.Backup) || recoveryWithin(change.Backup, change.Source) {
			return fmt.Errorf("unowned staging or backup path for %q", change.Target)
		}
	}

	return nil
}
