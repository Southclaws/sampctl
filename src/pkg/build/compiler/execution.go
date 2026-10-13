package compiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ExecutionError identifies a compiler process or output failure.
// Pawn source errors remain in the returned build problems.
type ExecutionError struct {
	Kind   string
	Path   string
	Output string
	Err    error
}

const (
	ExecutionMissing   = "missing"
	ExecutionStart     = "start"
	ExecutionCrash     = "crash"
	ExecutionCancelled = "cancelled"
	ExecutionOutput    = "output"
)

func (e *ExecutionError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("compiler %s failed (%s): %v\n%s", e.Path, e.Kind, e.Err, e.Output)
	}
	return fmt.Sprintf("compiler %s failed (%s): %v", e.Path, e.Kind, e.Err)
}

func (e *ExecutionError) Unwrap() error { return e.Err }

func lockCompilerExecution(ctx context.Context, cmd *exec.Cmd) (func() error, error) {
	dir := filepath.Dir(cmd.Path)
	for {
		_, markerErr := os.Lstat(filepath.Join(dir, compilerCompletionFilename))
		_, journalErr := os.Lstat(compilerJournalPath(dir))
		if markerErr == nil || journalErr == nil {
			if _, lockErr := os.Lstat(compilerLockPath(dir)); lockErr == nil {
				break
			} else if !errors.Is(lockErr, os.ErrNotExist) {
				return nil, lockErr
			}
		}
		if parent := filepath.Dir(dir); parent != dir {
			dir = parent
		} else {
			return func() error { return nil }, nil
		}
	}
	unlock, err := lockCompilerRead(ctx, dir)
	if err != nil {
		return nil, err
	}
	if journal, err := readCompilerJournal(dir); err != nil || journal != nil {
		if err == nil {
			err = errors.New("compiler installation has pending recovery")
		}
		return nil, errors.Join(&InstallationError{Path: dir, RecoveryPending: true, Err: err}, unlock())
	}
	completion, err := readCompilerCompletion(dir)
	if err == nil {
		err = verifyCompilerInstallation(ctx, dir, completion.Meta, completion.Platform, completion.Package)
	}
	if err != nil {
		return nil, errors.Join(&InstallationError{Path: dir, Err: err}, unlock())
	}
	return unlock, nil
}

func runCompilerCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case err := <-finished:
		return err
	case <-ctx.Done():
		killErr := cmd.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			killErr = nil
		}
		return errors.Join(ctx.Err(), killErr, <-finished)
	}
}
