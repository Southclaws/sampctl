package pkgcontext

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

// DependencyRecoveryFilename contains recovery state for the project dependency operation.
const DependencyRecoveryFilename = ".sampctl-dependencies.json"

// DependencyRecoveryStatus describes a pending operation without changing project files.
type DependencyRecoveryStatus struct {
	Pending, Committed, CanRecover, Active bool
	Restored                               bool
	Operation, Path                        string
	Stages                                 []string
}

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

// InspectDependencyRecovery reads and validates recovery state under the existing
// project lock. It does not create a lock file or repair an installation.
func InspectDependencyRecovery(ctx context.Context, project string) (status DependencyRecoveryStatus, err error) {
	status.Path = filepath.Join(project, DependencyRecoveryFilename)
	if err := ctx.Err(); err != nil {
		return status, err
	}
	lockPath := filepath.Join(project, ".sampctl-dependencies.lock")
	if err := regularRecoveryFile(lockPath); err != nil {
		return status, err
	}
	if _, err := os.Lstat(lockPath); err == nil {
		// Open read-only: inspection must not create or truncate project files.
		lock := flock.New(lockPath, flock.SetFlag(os.O_RDONLY))
		locked, err := lock.TryLock()
		if err != nil {
			return status, errors.Join(err, lock.Close())
		}
		if !locked {
			status.Active = true
			return status, lock.Close()
		}
		defer func() { err = errors.Join(err, lock.Unlock()) }()
	} else if !errors.Is(err, os.ErrNotExist) {
		return status, err
	}
	journal, err := readDependencyJournal(project)
	if err != nil {
		return status, err
	}
	if journal == nil {
		return status, nil
	}
	status = journal.status()
	if err := journal.validateState(ctx); err != nil {
		return status, err
	}
	status.CanRecover = true
	return status, nil
}

func (journal *dependencyJournal) status() DependencyRecoveryStatus {
	status := DependencyRecoveryStatus{Pending: true, Committed: journal.Phase == "committed", Restored: journal.Phase == "restored", Operation: journal.Operation, Path: journal.path(DependencyRecoveryFilename)}
	for _, stage := range journal.Stages {
		status.Stages = append(status.Stages, journal.path(stage.Path))
	}
	return status
}

func lockDependencyProject(ctx context.Context, project string) (*flock.Flock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project, err := filepath.Abs(project)
	if err != nil {
		return nil, &OperationError{Operation: "project", Err: err}
	}
	info, err := os.Lstat(project)
	if err != nil {
		return nil, &OperationError{Operation: "project", Err: fmt.Errorf("inspect project directory %s: %w", project, err)}
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, &OperationError{Operation: "project", Err: fmt.Errorf("project directory %s must be a directory, not a symbolic link or file", project)}
	}
	path := filepath.Join(project, ".sampctl-dependencies.lock")
	if err := regularRecoveryFile(path); err != nil {
		return nil, err
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil || !locked {
		if err == nil {
			err = errors.New("another sampctl dependency command is running in this project. Retry when it finishes")
		}
		return nil, &OperationError{Operation: "project", Err: errors.Join(err, lock.Close())}
	}
	return lock, nil
}

// RecoverDependencyOperation restores an interrupted dependency operation or
// retries committed cleanup under the same project lock used by dependency commands.
func RecoverDependencyOperation(ctx context.Context, project string) (status DependencyRecoveryStatus, err error) {
	// A project without recovery metadata needs no write, including a lock file.
	if _, err := os.Lstat(filepath.Join(project, DependencyRecoveryFilename)); errors.Is(err, os.ErrNotExist) {
		return status, ctx.Err()
	}
	lock, err := lockDependencyProject(ctx, project)
	if err != nil {
		return status, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return recoverDependencyOperationLocked(ctx, project)
}

func recoverDependencyOperationLocked(ctx context.Context, project string) (DependencyRecoveryStatus, error) {
	journal, err := readDependencyJournal(project)
	if err != nil {
		return DependencyRecoveryStatus{}, &OperationError{Operation: "recovery", Err: err}
	}
	if journal == nil {
		return DependencyRecoveryStatus{}, nil
	}
	status := journal.status()
	if err := journal.recover(ctx, os.Rename, os.RemoveAll); err != nil {
		outcome := OutcomeRollbackIncomplete
		if status.Committed {
			outcome = OutcomeCommitted
		} else if journal.Phase == "restored" && journal.validateState(context.Background()) == nil {
			outcome = OutcomeRolledBack
		}
		return status, &OperationError{Operation: "recovery", Outcome: outcome, Err: err}
	}
	status.CanRecover = true
	return status, nil
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
		if !validRecoveryHash(change.NewHash) || (change.OldHash != "" && !validRecoveryHash(change.OldHash)) || (change.OldHash == "") != (change.Backup == "") {
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

func validRecoveryHash(hash string) bool {
	data, err := hex.DecodeString(hash)
	return err == nil && len(data) == sha256.Size
}

func recoverySnapshotHash(file dependencyRecoveryFile) string {
	data := append([]byte(fmt.Sprintf("%s:%d:%t:", file.Path, file.Mode, file.Exists)), file.Data...)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func recoveryCleanupEntries(ctx context.Context, root string) ([]dependencyRecoveryCleanupEntry, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("cleanup path %s must be a directory", root)
	}
	var entries []dependencyRecoveryCleanupEntry
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		file := dependencyRecoveryCleanupEntry{Path: filepath.ToSlash(relative), Mode: uint32(info.Mode())}
		switch {
		case info.IsDir():
		case info.Mode()&os.ModeSymlink != 0:
			file.Link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		case info.Mode().IsRegular():
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, readErr := io.Copy(hash, dependencyContextReader{ctx: ctx, reader: f})
			if err := errors.Join(readErr, f.Close()); err != nil {
				return err
			}
			file.Hash = hex.EncodeToString(hash.Sum(nil))
		default:
			return fmt.Errorf("unsupported staged entry %s", path)
		}
		entries = append(entries, file)
		return nil
	})
	return entries, errors.Join(err, ctx.Err())
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

// This only inspects ancestors; recovery must never create a path while deciding
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

func recoveryTreeHash(ctx context.Context, path string, syncFiles bool) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("dependency recovery directory %s must be a directory", path)
	}
	hash := sha256.New()
	var directories []string
	err = filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(path, entryPath)
		if err != nil {
			return err
		}
		size := info.Size()
		if info.IsDir() {
			size = 0
		}
		if _, err := fmt.Fprintf(hash, "%q:%d:%d:", filepath.ToSlash(relative), info.Mode(), size); err != nil {
			return err
		}
		switch {
		case info.IsDir():
			directories = append(directories, entryPath)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(entryPath)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(hash, "%q", target); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			file, err := os.Open(entryPath)
			if err != nil {
				return err
			}
			_, readErr := io.Copy(hash, dependencyContextReader{ctx: ctx, reader: file})
			var syncErr error
			if syncFiles && readErr == nil {
				syncErr = syncDependencyRecoveryFile(file, info)
			}
			if err := errors.Join(readErr, syncErr, file.Close()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported dependency entry %s", entryPath)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if syncFiles {
		for i := len(directories) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if err := fs.SyncDir(directories[i]); err != nil {
				return "", err
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
		source, err := recoveryTreeHash(ctx, journal.path(change.Source), false)
		if err != nil {
			return err
		}
		backup := ""
		if change.Backup != "" {
			backup, err = recoveryTreeHash(ctx, journal.path(change.Backup), false)
			if err != nil {
				return err
			}
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
			if target != change.OldHash || backup != "" {
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

func renameRecoveryPath(rename func(string, string) error, from, to string) error {
	if err := rename(from, to); err != nil {
		return err
	}
	return errors.Join(fs.SyncDir(filepath.Dir(from)), fs.SyncDir(filepath.Dir(to)))
}

func (journal *dependencyJournal) recover(ctx context.Context, rename func(string, string) error, removeAll func(string) error) error {
	if err := journal.validateState(ctx); err != nil {
		return err
	}
	if journal.Phase == "active" {
		for i := len(journal.Changes) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return err
			}
			change := journal.Changes[i]
			target := journal.path(change.Target)
			backup := journal.path(change.Backup)
			if change.Backup != "" {
				if _, err := os.Lstat(backup); errors.Is(err, os.ErrNotExist) {
					continue // Never moved, or already restored on a previous attempt.
				} else if err != nil {
					return err
				}
			}
			if _, err := os.Lstat(target); err == nil {
				// Move the known replacement back to its recorded source. Avoid
				// destructive removal so another interruption retains both copies.
				if err := renameRecoveryPath(rename, target, journal.path(change.Source)); err != nil {
					return fmt.Errorf("retain replacement %s before restoring backup %s: %w", target, backup, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if change.Backup != "" {
				if err := renameRecoveryPath(rename, backup, target); err != nil {
					return fmt.Errorf("restore dependency %s from %s: %w", target, backup, err)
				}
			}
		}
		for _, file := range journal.Files {
			if err := ctx.Err(); err != nil {
				return err
			}
			snapshot := projectFileSnapshot{path: journal.path(file.Path), data: file.Data, mode: os.FileMode(file.Mode), exists: file.Exists}
			if err := snapshot.restore(); err != nil {
				return fmt.Errorf("restore project file %s: %w", snapshot.path, err)
			}
			if err := fs.SyncDir(journal.project); err != nil {
				return err
			}
		}
		journal.Phase = "restored"
		if err := journal.save(); err != nil {
			return err
		}
	}
	for i := range journal.Stages {
		stage := &journal.Stages[i]
		if err := ctx.Err(); err != nil {
			return err
		}
		if !stage.Cleanup {
			entries, err := recoveryCleanupEntries(ctx, journal.path(stage.Path))
			if err != nil {
				return err
			}
			// Missing entries are safe after partial deletion; changed or new entries
			// must remain distinguishable from the owned files scheduled for cleanup.
			stage.CleanupEntries = entries
			stage.Cleanup = true
			if err := journal.save(); err != nil {
				return err
			}
		}
		if err := removeAll(journal.path(stage.Path)); err != nil {
			return fmt.Errorf("clean dependency staging directory %s: %w", journal.path(stage.Path), err)
		}
		if err := fs.SyncDir(filepath.Dir(journal.path(stage.Path))); err != nil {
			return err
		}
	}
	if err := os.Remove(journal.path(DependencyRecoveryFilename)); err != nil {
		return fmt.Errorf("remove dependency recovery record: %w", err)
	}
	return fs.SyncDir(journal.project)
}
