package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

const (
	RuntimeRecoveryFilename = ".sampctl-runtime-recovery.json"
	runtimeLockFilename     = ".sampctl-runtime.lock"
	runtimeOperationPrefix  = ".sampctl-runtime-operation-"
)

// RuntimeInstallError reports the verified outcome of a runtime update.
type RuntimeInstallError struct {
	Runtime         string
	Restored        bool
	Committed       bool
	RecoveryPending bool
	Err             error
}

func (e *RuntimeInstallError) Error() string {
	return fmt.Sprintf("install runtime %s: %v", e.Runtime, e.Err)
}

func (e *RuntimeInstallError) Unwrap() error { return e.Err }

func runtimeRecoveryFailure(root, version string, err error) *RuntimeInstallError {
	failure := &RuntimeInstallError{Runtime: version, RecoveryPending: true, Err: err}
	journal, readErr := readRuntimeJournal(root)
	if readErr == nil {
		failure.Runtime = journal.Current.Version
		failure.Committed = journal.Phase == "committed"
		failure.Restored = journal.Phase == "restored" && journal.Previous != nil && verifyRuntimeManifest(*journal.Previous, root) == nil
	}

	return failure
}

// RuntimeRecoveryStatus describes an interrupted update without changing files.
type RuntimeRecoveryStatus struct {
	Pending    bool
	Active     bool
	Committed  bool
	Restored   bool
	CanRecover bool
	Runtime    string
	Path       string
}

type runtimeJournal struct {
	Format   int                   `json:"format"`
	Phase    string                `json:"phase"`
	Stage    string                `json:"stage"`
	Previous *runtimeManifest      `json:"previous,omitempty"`
	Current  runtimeManifest       `json:"current"`
	Files    []runtimeJournalEntry `json:"files"`
}

type runtimeJournalEntry struct {
	Path   string           `json:"path"`
	Before *runtimeFileInfo `json:"before,omitempty"`
	After  *runtimeFileInfo `json:"after,omitempty"`
}

func runtimeStageFile(root string, journal *runtimeJournal, index int, kind string) string {
	return filepath.Join(root, journal.Stage, fmt.Sprintf("%s-%06d", kind, index))
}

func runtimeBeforePlatform(journal *runtimeJournal, entry runtimeJournalEntry) string {
	if journal.Previous != nil {
		for _, file := range journal.Previous.Files {
			if strings.EqualFold(file.Path, entry.Path) {
				return journal.Previous.Platform
			}
		}
	}

	return journal.Current.Platform
}

func syncRuntimeParents(root, path string) error {
	root = filepath.Clean(root)
	for {
		if err := fs.SyncDir(path); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fmt.Errorf("runtime directory is outside its installation: %w", os.ErrInvalid)
		}
		path = parent
	}
}

func lockRuntime(ctx context.Context, root string) (*flock.Flock, error) {
	return lockRuntimePath(ctx, root, runtimeLockFilename)
}

func lockRuntimePath(ctx context.Context, root, filename string) (*flock.Flock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if root == "" {
		return nil, fmt.Errorf("runtime installation directory is empty: %w", os.ErrInvalid)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	if _, err := runtimeFilePath(root, filename); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	lock := flock.New(filepath.Join(root, filename))
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil || !locked {
		if err == nil {
			err = ctx.Err()
		}
		return nil, errors.Join(err, lock.Close())
	}

	return lock, nil
}

// InspectRuntimeRecovery checks pending recovery without creating files.
func InspectRuntimeRecovery(ctx context.Context, root string) (status RuntimeRecoveryStatus, err error) {
	if err := ctx.Err(); err != nil {
		return status, err
	}

	path := filepath.Join(root, RuntimeRecoveryFilename)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return status, nil
	} else if err != nil {
		return status, err
	}

	status = RuntimeRecoveryStatus{Pending: true, Path: path}
	lockPath, err := runtimeFilePath(root, runtimeLockFilename)
	if err != nil {
		return status, err
	}
	if _, statErr := os.Lstat(lockPath); statErr == nil {
		lock := flock.New(lockPath, flock.SetFlag(os.O_RDONLY))
		locked, lockErr := lock.TryRLock()
		if lockErr != nil || !locked {
			status.Active = !locked && lockErr == nil
			return status, errors.Join(fmt.Errorf("runtime update is still running"), lockErr, lock.Close())
		}
		defer func() { err = errors.Join(err, lock.Close()) }()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return status, statErr
	}

	journal, err := readRuntimeJournal(root)
	if err != nil {
		return status, err
	}

	status.Committed = journal.Phase == "committed"
	status.Restored = journal.Phase == "restored"
	status.Runtime = journal.Current.Version
	if err := validateRuntimeRecoveryState(root, journal); err != nil {
		return status, err
	}
	status.CanRecover = true

	return status, nil
}

// RecoverRuntime restores an interrupted update or finishes committed cleanup.
func RecoverRuntime(ctx context.Context, root string) (err error) {
	if _, err := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename)); errors.Is(err, os.ErrNotExist) {
		return ctx.Err()
	}

	lock, err := lockRuntime(ctx, root)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()

	return recoverRuntimeLocked(ctx, root)
}

// VerifyInstalledRuntime verifies tracked runtime files without changing them.
func VerifyInstalledRuntime(root string) error {
	manifest, err := loadInstalledRuntimeManifest(root)
	if err != nil {
		return err
	}

	if manifest == nil {
		return fmt.Errorf("runtime is not installed: %w", os.ErrNotExist)
	}

	return verifyRuntimeManifest(*manifest, root)
}

func readRuntimeJournal(root string) (*runtimeJournal, error) {
	path, err := runtimeFilePath(root, RuntimeRecoveryFilename)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var journal runtimeJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return nil, fmt.Errorf("read runtime recovery state: %w", err)
	}

	if err := validateRuntimeJournal(root, &journal); err != nil {
		return nil, err
	}

	return &journal, nil
}

func validateRuntimeJournal(root string, journal *runtimeJournal) error {
	if journal.Format != 1 || (journal.Phase != "pending" && journal.Phase != "committed" && journal.Phase != "restored") || !strings.HasPrefix(journal.Stage, runtimeOperationPrefix) || filepath.Base(journal.Stage) != journal.Stage || len(journal.Files) == 0 {
		return fmt.Errorf("runtime recovery state is invalid: %w", os.ErrInvalid)
	}

	if err := validateInstalledRuntimeManifest(journal.Current, root); err != nil {
		return err
	}

	if journal.Previous != nil {
		if err := validateInstalledRuntimeManifest(*journal.Previous, root); err != nil {
			return err
		}
	}

	current := make(map[string]runtimeFileInfo, len(journal.Current.Files))
	previous := make(map[string]runtimeFileInfo)
	for _, file := range journal.Current.Files {
		current[strings.ToLower(file.Path)] = file
	}
	if journal.Previous != nil {
		for _, file := range journal.Previous.Files {
			previous[strings.ToLower(file.Path)] = file
		}
	}

	seen := make(map[string]bool, len(journal.Files))
	for _, entry := range journal.Files {
		if _, err := runtimeFilePath(root, entry.Path); err != nil {
			return err
		}

		key := strings.ToLower(entry.Path)
		if seen[key] || (entry.Before == nil && entry.After == nil) {
			return fmt.Errorf("invalid runtime recovery file %s: %w", entry.Path, os.ErrInvalid)
		}
		seen[key] = true

		for _, file := range []*runtimeFileInfo{entry.Before, entry.After} {
			if file != nil {
				candidate := journal.Current
				candidate.Files = []runtimeFileInfo{*file}
				// The real path was checked above. This temporary path lets the shared
				// validator also check fingerprints for the reserved ownership record.
				candidate.Files[0].Path = "fingerprint"
				if file.Path != entry.Path {
					return fmt.Errorf("invalid runtime recovery fingerprint: %w", os.ErrInvalid)
				}
				if err := validateInstalledRuntimeManifest(candidate, root); err != nil {
					return err
				}
			}
		}

		if entry.Path == installedRuntimeManifestFileName {
			if entry.After == nil {
				return fmt.Errorf("runtime recovery ownership is missing: %w", os.ErrInvalid)
			}

			data, err := json.MarshalIndent(journal.Current, "", "  ")
			if err != nil {
				return err
			}
			hash := sha256.Sum256(data)
			if entry.After.Hash != hex.EncodeToString(hash[:]) || entry.After.Size != int64(len(data)) || entry.After.Mode != 0o600 {
				return fmt.Errorf("runtime recovery ownership does not match the replacement: %w", os.ErrInvalid)
			}
			continue
		}

		if entry.After != nil && current[key] != *entry.After {
			return fmt.Errorf("runtime recovery replacement is not tracked: %w", os.ErrInvalid)
		}
		if _, retained := current[key]; retained && entry.After == nil {
			return fmt.Errorf("runtime recovery removes a required file: %w", os.ErrInvalid)
		}
		if entry.Before != nil && previous[key] != *entry.Before && current[key] != *entry.Before {
			return fmt.Errorf("runtime recovery backup is not tracked: %w", os.ErrInvalid)
		}
	}

	if !seen[installedRuntimeManifestFileName] {
		return fmt.Errorf("runtime recovery ownership is missing: %w", os.ErrInvalid)
	}

	for key := range current {
		if !seen[key] {
			return fmt.Errorf("runtime recovery file list is incomplete: %w", os.ErrInvalid)
		}
	}

	return nil
}

func runtimeFileMatches(root, relative, platform string, expected *runtimeFileInfo) (bool, error) {
	path, err := runtimeFilePath(root, relative)
	if err != nil {
		return false, err
	}

	_, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return expected == nil, nil
	}
	if err != nil {
		return false, err
	}

	if expected == nil {
		return false, nil
	}

	file := *expected
	file.Path = relative
	return verifyRuntimeManifest(runtimeManifest{Platform: platform, Files: []runtimeFileInfo{file}}, root) == nil, nil
}

func validateRuntimeRecoveryState(root string, journal *runtimeJournal) error {
	if journal.Phase == "committed" {
		if err := verifyRuntimeManifest(journal.Current, root); err != nil {
			return err
		}
	}

	stage := filepath.Join(root, journal.Stage)
	info, err := os.Lstat(stage)
	if err != nil && !(errors.Is(err, os.ErrNotExist) && journal.Phase != "pending") {
		return err
	}
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("runtime recovery directory is unsafe: %w", os.ErrInvalid)
	}

	expectedStage := make(map[string]*runtimeFileInfo, len(journal.Files)*2)
	for index, entry := range journal.Files {
		for _, kind := range []string{"old", "new"} {
			file := entry.Before
			if kind == "new" {
				file = entry.After
			}
			if file != nil {
				expectedStage[fmt.Sprintf("%s-%06d", kind, index)] = file
			}
		}

		before, err := runtimeFileMatches(root, entry.Path, runtimeBeforePlatform(journal, entry), entry.Before)
		if err != nil {
			return err
		}

		after, err := runtimeFileMatches(root, entry.Path, journal.Current.Platform, entry.After)
		if err != nil {
			return err
		}
		if (journal.Phase == "pending" && !before && !after) || (journal.Phase == "committed" && !after) || (journal.Phase == "restored" && !before) {
			return fmt.Errorf("runtime file %s changed during recovery: %w", entry.Path, os.ErrInvalid)
		}

		if journal.Phase == "pending" && entry.Before != nil {
			match, err := runtimeFileMatches(stage, fmt.Sprintf("old-%06d", index), runtimeBeforePlatform(journal, entry), entry.Before)
			if err != nil || !match {
				return errors.Join(fmt.Errorf("runtime backup for %s is missing or changed: %w", entry.Path, os.ErrInvalid), err)
			}
		}

		if entry.Path == installedRuntimeManifestFileName && entry.Before != nil {
			path := runtimeStageFile(root, journal, index, "old")
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) && journal.Phase == "restored" {
				path = filepath.Join(root, installedRuntimeManifestFileName)
			}
			previous, err := readRuntimeManifest(path)
			if journal.Phase == "committed" && errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil || journal.Previous == nil || !manifestsEqual(previous, *journal.Previous) {
				return errors.Join(fmt.Errorf("runtime ownership backup does not match the previous installation: %w", os.ErrInvalid), err)
			}
		}
	}

	if info != nil {
		entries, err := os.ReadDir(stage)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			expected := expectedStage[entry.Name()]
			if expected == nil {
				return fmt.Errorf("unknown file in runtime recovery directory: %s: %w", entry.Name(), os.ErrInvalid)
			}

			match, err := runtimeFileMatches(stage, entry.Name(), journal.Current.Platform, expected)
			if err != nil || !match {
				return errors.Join(fmt.Errorf("runtime recovery file %s changed: %w", entry.Name(), os.ErrInvalid), err)
			}
		}
	}

	return nil
}

func recoverRuntimeLocked(ctx context.Context, root string) error {
	journal, err := readRuntimeJournal(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if err := validateRuntimeRecoveryState(root, journal); err != nil {
		return err
	}

	if journal.Phase == "pending" {
		for index := len(journal.Files) - 1; index >= 0; index-- {
			if err := ctx.Err(); err != nil {
				return err
			}

			entry := journal.Files[index]
			match, err := runtimeFileMatches(root, entry.Path, runtimeBeforePlatform(journal, entry), entry.Before)
			if err != nil {
				return err
			}
			if match {
				continue
			}

			match, err = runtimeFileMatches(root, entry.Path, journal.Current.Platform, entry.After)
			if err != nil || !match {
				return errors.Join(fmt.Errorf("runtime file %s changed during recovery: %w", entry.Path, os.ErrInvalid), err)
			}

			path, err := runtimeFilePath(root, entry.Path)
			if err != nil {
				return err
			}

			if entry.Before == nil {
				if err := os.Remove(path); err != nil {
					return err
				}
			} else if err := copyFileWithModeContext(ctx, runtimeStageFile(root, journal, index, "old"), path, os.FileMode(entry.Before.Mode)); err != nil {
				return err
			}

			if err := fs.SyncDir(filepath.Dir(path)); err != nil {
				return err
			}
		}

		journal.Phase = "restored"
		if err := validateRuntimeRecoveryState(root, journal); err != nil {
			return err
		}

		if err := writeRuntimeJournal(root, journal); err != nil {
			return err
		}
	}

	return cleanupRuntimeJournal(ctx, root, journal)
}

func writeRuntimeJournal(root string, journal *runtimeJournal) error {
	return fs.WriteJSONAtomic(filepath.Join(root, RuntimeRecoveryFilename), journal, 0o755, 0o600)
}

func cleanupRuntimeJournal(ctx context.Context, root string, journal *runtimeJournal) error {
	return cleanupRuntimeJournalWithHooks(ctx, root, journal, runtimeInstallHooks{})
}

func cleanupRuntimeJournalWithHooks(ctx context.Context, root string, journal *runtimeJournal, hooks runtimeInstallHooks) error {
	if err := validateRuntimeRecoveryState(root, journal); err != nil {
		return err
	}

	stage := filepath.Join(root, journal.Stage)
	for index, entry := range journal.Files {
		for _, kind := range []string{"old", "new"} {
			if err := ctx.Err(); err != nil {
				return err
			}

			file := entry.Before
			if kind == "new" {
				file = entry.After
			}
			if file == nil {
				continue
			}

			path := runtimeStageFile(root, journal, index, kind)
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
				continue
			}

			match, err := runtimeFileMatches(stage, filepath.Base(path), journal.Current.Platform, file)
			if err != nil || !match {
				return errors.Join(fmt.Errorf("runtime recovery file changed before cleanup: %w", os.ErrInvalid), err)
			}

			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			if err := hooks.checkpoint("cleanup:" + filepath.Base(path)); err != nil {
				return err
			}
		}
	}

	if err := os.Remove(stage); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := fs.SyncDir(root); err != nil {
		return err
	}

	if err := hooks.checkpoint("cleanup:stage"); err != nil {
		return err
	}

	if err := os.Remove(filepath.Join(root, RuntimeRecoveryFilename)); err != nil {
		return err
	}

	return fs.SyncDir(root)
}
