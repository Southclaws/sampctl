package compiler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	infrafs "github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type compilerJournal struct {
	Version    int                    `json:"format"`
	Stage      string                 `json:"stage"`
	Backup     string                 `json:"backup,omitempty"`
	OldHash    string                 `json:"old_sha256,omitempty"`
	NewHash    string                 `json:"new_sha256"`
	Committed  bool                   `json:"committed"`
	Cleaning   bool                   `json:"cleaning,omitempty"`
	NewEntries []compilerCleanupEntry `json:"new_entries"`
	OldEntries []compilerCleanupEntry `json:"old_entries,omitempty"`
}

type compilerCleanupEntry struct {
	Path      string `json:"path"`
	Hash      string `json:"sha256,omitempty"`
	Mode      uint32 `json:"mode,omitempty"`
	Directory bool   `json:"directory,omitempty"`
}

func compilerJournalPath(dir string) string {
	return filepath.Join(filepath.Dir(dir), "."+filepath.Base(dir)+".sampctl-compiler-install.json")
}

func readCompilerJournal(dir string) (*compilerJournal, error) {
	path := compilerJournalPath(dir)
	if _, err := compilerRegularFile(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var journal compilerJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return nil, fmt.Errorf("invalid compiler recovery metadata: %w", err)
	}
	prefix := "." + filepath.Base(dir)
	if journal.Version != 1 || !compilerRelativePath(journal.Stage) || filepath.Base(journal.Stage) != journal.Stage || !strings.HasPrefix(journal.Stage, prefix+".stage-") || len(journal.NewHash) != sha256.Size*2 {
		return nil, errors.New("invalid compiler recovery metadata")
	}
	if _, err := hex.DecodeString(journal.NewHash); err != nil {
		return nil, errors.New("invalid staged compiler fingerprint")
	}
	if journal.Backup != "" {
		if !compilerRelativePath(journal.Backup) || filepath.Base(journal.Backup) != journal.Backup || !strings.HasPrefix(journal.Backup, prefix+".backup-") || len(journal.OldHash) != sha256.Size*2 {
			return nil, errors.New("invalid compiler backup metadata")
		}
		if _, err := hex.DecodeString(journal.OldHash); err != nil {
			return nil, errors.New("invalid compiler backup fingerprint")
		}
	} else if journal.OldHash != "" {
		return nil, errors.New("compiler backup metadata is missing")
	}
	if err := validateCompilerCleanupEntries(journal.NewEntries); err != nil {
		return nil, err
	}
	if journal.OldHash != "" {
		if err := validateCompilerCleanupEntries(journal.OldEntries); err != nil {
			return nil, err
		}
	}
	return &journal, nil
}

func compilerTreeHash(ctx context.Context, dir string) (string, error) {
	files, err := compilerInventory(ctx, dir, true)
	if err != nil {
		return "", err
	}
	var directories []string
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			directories = append(directories, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(struct {
		Files []compilerFile
		Dirs  []string
	}{files, directories})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func installCompilerArchive(ctx context.Context, archive, dir string, meta versioning.DependencyMeta, platform string, pkg download.Compiler) error {
	return installCompilerArchiveWithHook(ctx, archive, dir, meta, platform, pkg, nil)
}

func installCompilerArchiveWithHook(ctx context.Context, archive, dir string, meta versioning.DependencyMeta, platform string, pkg download.Compiler, hook func(string) error) (err error) {
	if err := validateCompilerPackage(pkg); err != nil {
		return err
	}
	if err := compilerSafePath(dir, true); err != nil {
		return &InstallationError{Path: dir, Err: err}
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), "."+filepath.Base(dir)+".stage-")
	if err != nil {
		return err
	}
	journalWritten := false
	defer func() {
		if !journalWritten {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	if _, err := extractCompilerArchive(ctx, archive, stage, pkg); err != nil {
		return &InstallationError{Path: dir, Err: &compilerArchiveError{Path: archive, Err: err}}
	}
	if err := validateCompilerFiles(stage, pkg); err != nil {
		return &InstallationError{Path: dir, Err: &compilerArchiveError{Path: archive, Err: fmt.Errorf("compiler archive is incomplete: %w", err)}}
	}
	// Record the permissions actually supported by the host filesystem.
	files, err := compilerInventory(ctx, stage, false)
	if err != nil {
		return err
	}
	completion := compilerCompletion{Version: 1, Meta: meta, Platform: platform, Package: pkg, Files: files}
	if err := infrafs.WriteJSONAtomic(filepath.Join(stage, compilerCompletionFilename), completion, 0o700, 0o600); err != nil {
		return err
	}
	if err := verifyCompilerInstallation(ctx, stage, meta, platform, pkg); err != nil {
		return &InstallationError{Path: dir, Err: err}
	}
	if err := compilerReplacementAllowed(ctx, dir, files); err != nil {
		return &InstallationError{Path: dir, Err: err}
	}
	if err := syncCompilerTree(ctx, stage); err != nil {
		return err
	}
	newHash, err := compilerTreeHash(ctx, stage)
	if err != nil {
		return err
	}
	journal := &compilerJournal{Version: 1, Stage: filepath.Base(stage), NewHash: newHash}
	journal.NewEntries, err = compilerCleanupEntries(ctx, stage)
	if err != nil {
		return err
	}
	if _, statErr := os.Lstat(dir); statErr == nil {
		journal.OldHash, err = compilerTreeHash(ctx, dir)
		if err != nil {
			return err
		}
		if journal.OldHash == journal.NewHash {
			return nil
		}
		journal.OldEntries, err = compilerCleanupEntries(ctx, dir)
		if err != nil {
			return err
		}
		backup, err := os.MkdirTemp(filepath.Dir(dir), "."+filepath.Base(dir)+".backup-")
		if err != nil {
			return err
		}
		if err := os.Remove(backup); err != nil {
			return err
		}
		journal.Backup = filepath.Base(backup)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := infrafs.WriteJSONAtomic(compilerJournalPath(dir), journal, 0o700, 0o600); err != nil {
		// Atomic persistence can succeed before directory synchronization fails.
		if _, statErr := os.Lstat(compilerJournalPath(dir)); statErr == nil {
			journalWritten = true
		}
		return &InstallationError{Path: dir, RecoveryPending: journalWritten, Err: err}
	}
	journalWritten = true
	transition := func(point string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if hook != nil {
			return hook(point)
		}
		return nil
	}
	apply := func() error {
		if err := transition("prepared"); err != nil {
			return err
		}
		if journal.Backup != "" {
			if err := os.Rename(dir, filepath.Join(filepath.Dir(dir), journal.Backup)); err != nil {
				return err
			}
			if err := infrafs.SyncDir(filepath.Dir(dir)); err != nil {
				return err
			}
		}
		if err := transition("backed-up"); err != nil {
			return err
		}
		if err := os.Rename(stage, dir); err != nil {
			return err
		}
		if err := infrafs.SyncDir(filepath.Dir(dir)); err != nil {
			return err
		}
		if err := transition("promoted"); err != nil {
			return err
		}
		if err := verifyCompilerInstallation(ctx, dir, meta, platform, pkg); err != nil {
			return err
		}
		journal.Committed = true
		if err := infrafs.WriteJSONAtomic(compilerJournalPath(dir), journal, 0o700, 0o600); err != nil {
			return err
		}
		return transition("committed")
	}
	applyErr := apply()
	// Recovery uses the persisted commit state, including a failed final sync.
	persisted, journalErr := readCompilerJournal(dir)
	committed := persisted != nil && persisted.Committed
	recoveryErr := recoverCompilerInstallationWithHook(context.WithoutCancel(ctx), dir, hook)
	if err := errors.Join(applyErr, journalErr, recoveryErr); err != nil {
		_, pendingErr := os.Lstat(compilerJournalPath(dir))
		restored := false
		if !committed && journal.OldHash != "" && recoveryErr == nil {
			actual, verifyErr := compilerTreeHash(context.WithoutCancel(ctx), dir)
			restored = verifyErr == nil && actual == journal.OldHash
		}
		return &InstallationError{Path: dir, Restored: restored, Committed: committed, RecoveryPending: pendingErr == nil, Err: err}
	}
	return verifyCompilerInstallation(ctx, dir, meta, platform, pkg)
}

func syncCompilerTree(ctx context.Context, dir string) error {
	var directories []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		return errors.Join(file.Sync(), file.Close())
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := infrafs.SyncDir(directories[i]); err != nil {
			return err
		}
	}
	return nil
}

type compilerRecoveryState struct {
	Destination string
	Stage       string
	Backup      string
}

func inspectCompilerRecovery(ctx context.Context, dir string, journal *compilerJournal) (compilerRecoveryState, error) {
	var state compilerRecoveryState
	locations := []struct {
		path string
		hash *string
	}{
		{dir, &state.Destination},
		{filepath.Join(filepath.Dir(dir), journal.Stage), &state.Stage},
	}
	if journal.Backup != "" {
		locations = append(locations, struct {
			path string
			hash *string
		}{filepath.Join(filepath.Dir(dir), journal.Backup), &state.Backup})
	}
	for _, location := range locations {
		hash, err := compilerTreeHash(ctx, location.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return state, err
		}
		*location.hash = hash
	}
	if state.Stage != "" {
		if journal.Cleaning {
			if err := validateRemainingCompilerEntries(ctx, filepath.Join(filepath.Dir(dir), journal.Stage), journal.NewEntries); err != nil {
				return state, err
			}
		} else if state.Stage != journal.NewHash {
			return state, errors.New("staged compiler changed after installation was interrupted")
		}
	}
	if state.Backup != "" {
		if journal.Committed && journal.Cleaning {
			if err := validateRemainingCompilerEntries(ctx, filepath.Join(filepath.Dir(dir), journal.Backup), journal.OldEntries); err != nil {
				return state, err
			}
		} else if state.Backup != journal.OldHash {
			return state, errors.New("compiler backup changed after installation was interrupted")
		}
	}
	if state.Destination != "" && state.Destination != journal.NewHash && state.Destination != journal.OldHash {
		return state, errors.New("compiler installation changed after installation was interrupted")
	}
	if journal.Committed {
		if state.Destination != journal.NewHash {
			return state, errors.New("committed compiler installation is missing or modified")
		}
	} else if journal.OldHash != "" && state.Destination != journal.OldHash && state.Backup != journal.OldHash {
		return state, errors.New("previous compiler installation is unavailable for recovery")
	}
	return state, nil
}

func validateCompilerRecovery(ctx context.Context, dir string, journal *compilerJournal) error {
	_, err := inspectCompilerRecovery(ctx, dir, journal)
	return err
}

func recoverCompilerInstallation(ctx context.Context, dir string) error {
	return recoverCompilerInstallationWithHook(ctx, dir, nil)
}

func recoverCompilerInstallationWithHook(ctx context.Context, dir string, hook func(string) error) error {
	journal, err := readCompilerJournal(dir)
	if err != nil || journal == nil {
		return err
	}
	state, err := inspectCompilerRecovery(ctx, dir, journal)
	if err != nil {
		return &InstallationError{Path: dir, Err: err}
	}
	parent := filepath.Dir(dir)
	if !journal.Committed {
		if state.Destination == journal.NewHash {
			if state.Stage != "" {
				return errors.New("compiler replacement and staging directory both exist")
			}
			if err := os.Rename(dir, filepath.Join(parent, journal.Stage)); err != nil {
				return err
			}
			state.Stage = journal.NewHash
			state.Destination = ""
		}
		if state.Destination == "" && state.Backup != "" {
			if err := os.Rename(filepath.Join(parent, journal.Backup), dir); err != nil {
				return err
			}
			state.Backup = ""
		}
	}
	if !journal.Cleaning {
		journal.Cleaning = true
		if err := infrafs.WriteJSONAtomic(compilerJournalPath(dir), journal, 0o700, 0o600); err != nil {
			return err
		}
	}
	if state.Stage != "" {
		if err := cleanupCompilerEntries(ctx, filepath.Join(parent, journal.Stage), journal.NewEntries, hook); err != nil {
			return err
		}
	}
	if state.Backup != "" {
		if err := cleanupCompilerEntries(ctx, filepath.Join(parent, journal.Backup), journal.OldEntries, hook); err != nil {
			return err
		}
	}
	if err := infrafs.SyncDir(parent); err != nil {
		return err
	}
	if err := os.Remove(compilerJournalPath(dir)); err != nil {
		return err
	}
	return infrafs.SyncDir(parent)
}

func compilerCleanupEntries(ctx context.Context, dir string) ([]compilerCleanupEntry, error) {
	files, err := compilerInventory(ctx, dir, true)
	if err != nil {
		return nil, err
	}
	entries := make([]compilerCleanupEntry, 0, len(files))
	for _, file := range files {
		entries = append(entries, compilerCleanupEntry{Path: file.Path, Hash: file.Hash, Mode: file.Mode})
	}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != dir && entry.IsDir() {
			relative, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			entries = append(entries, compilerCleanupEntry{Path: filepath.ToSlash(relative), Directory: true})
		}
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, err
}

func validateCompilerCleanupEntries(entries []compilerCleanupEntry) error {
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !compilerRelativePath(entry.Path) || seen[entry.Path] {
			return errors.New("invalid compiler cleanup inventory")
		}
		if !entry.Directory {
			if len(entry.Hash) != sha256.Size*2 {
				return errors.New("invalid compiler cleanup fingerprint")
			}
			if _, err := hex.DecodeString(entry.Hash); err != nil {
				return errors.New("invalid compiler cleanup fingerprint")
			}
		}
		seen[entry.Path] = true
	}
	return nil
}

func validateRemainingCompilerEntries(ctx context.Context, dir string, expected []compilerCleanupEntry) error {
	actual, err := compilerCleanupEntries(ctx, dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	allowed := make(map[string]compilerCleanupEntry, len(expected))
	for _, entry := range expected {
		allowed[entry.Path] = entry
	}
	for _, entry := range actual {
		if previous, ok := allowed[entry.Path]; !ok || !reflect.DeepEqual(previous, entry) {
			return fmt.Errorf("compiler cleanup contains changed or unknown content %s", entry.Path)
		}
	}
	return nil
}

func cleanupCompilerEntries(ctx context.Context, dir string, entries []compilerCleanupEntry, hook func(string) error) error {
	if err := validateRemainingCompilerEntries(ctx, dir, entries); err != nil {
		return err
	}
	ordered := append([]compilerCleanupEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Directory != ordered[j].Directory {
			return !ordered[i].Directory
		}
		return len(ordered[i].Path) > len(ordered[j].Path)
	})
	for _, entry := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyCompilerCleanupEntry(ctx, dir, entry); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(dir, entry.Path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if hook != nil {
			if err := hook("cleanup-entry"); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func verifyCompilerCleanupEntry(ctx context.Context, dir string, entry compilerCleanupEntry) error {
	path := filepath.Join(dir, filepath.FromSlash(entry.Path))
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if entry.Directory {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("compiler cleanup directory changed %s", entry.Path)
		}
		return nil
	}
	if _, err := compilerRegularFile(path); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	current, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, current) {
		return errors.Join(errors.New("compiler cleanup file changed"), statErr, file.Close())
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, &compilerContextReader{ctx: ctx, reader: file})
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != entry.Hash || uint32(current.Mode().Perm()) != entry.Mode {
		return fmt.Errorf("compiler cleanup file changed %s", entry.Path)
	}
	return nil
}
