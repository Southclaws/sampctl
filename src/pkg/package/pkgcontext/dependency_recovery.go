package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

// DependencyRecoveryStatus describes a pending operation without changing project files.
type DependencyRecoveryStatus struct {
	Pending, Committed, CanRecover, Active bool
	Restored                               bool
	Operation, Path                        string
	Stages                                 []string
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

			// Missing entries are safe after partial deletion. Changed or new entries
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
