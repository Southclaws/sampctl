package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	copydir "github.com/otiai10/copy"
)

type dependencyInstallation struct {
	target   string
	backup   string
	promoted bool
}

type dependencyInstallState struct {
	installations []dependencyInstallation
	stages        []string
	rename        func(string, string) error
	removeAll     func(string) error
}

func newDependencyInstallState() *dependencyInstallState {
	return &dependencyInstallState{rename: os.Rename, removeAll: os.RemoveAll}
}

func (state *dependencyInstallState) stage(vendor string) (string, error) {
	if err := ensureManagedDirectory(vendor); err != nil {
		return "", err
	}
	path, err := os.MkdirTemp(vendor, ".sampctl-stage-")
	if err != nil {
		return "", fmt.Errorf("create dependency staging directory: %w", err)
	}
	state.stages = append(state.stages, path)
	return path, nil
}

func (state *dependencyInstallState) promote(source, target string) error {
	stage, err := state.stageForSource(source)
	if err != nil {
		return err
	}
	if err := ensureManagedDirectoryWithin(stage, filepath.Dir(source)); err != nil {
		return err
	}
	if err := ensureManagedDirectoryWithin(filepath.Dir(stage), filepath.Dir(target)); err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect dependency destination %s: %w", target, err)
	}
	change := dependencyInstallation{target: target}
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("dependency destination %s must be a directory, not a symbolic link or file", target)
		}
		backup, err := os.MkdirTemp(filepath.Dir(source), ".previous-")
		if err != nil {
			return fmt.Errorf("prepare dependency backup: %w", err)
		}
		change.backup = filepath.Join(backup, "dependency")
		if err := state.rename(target, change.backup); err != nil {
			return fmt.Errorf("back up dependency %s: %w", target, err)
		}
	}
	state.installations = append(state.installations, change)
	if err := state.rename(source, target); err != nil {
		return fmt.Errorf("promote dependency %s: %w", target, err)
	}
	state.installations[len(state.installations)-1].promoted = true
	return nil
}

func (state *dependencyInstallState) stageForSource(source string) (string, error) {
	for _, stage := range state.stages {
		relative, err := filepath.Rel(stage, source)
		if err == nil && relative != "." && filepath.IsLocal(relative) {
			return stage, nil
		}
	}
	return "", fmt.Errorf("dependency source is outside owned staging directories: %s", source)
}

func ensureManagedDirectoryWithin(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("dependency directory %s is outside managed directory %s", path, root)
	}
	if err := ensureManagedDirectory(root); err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		if err := ensureManagedDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func (state *dependencyInstallState) rollback() error {
	var failures []error
	for i := len(state.installations) - 1; i >= 0; i-- {
		change := state.installations[i]
		if change.promoted {
			if err := state.removeAll(change.target); err != nil {
				failures = append(failures, fmt.Errorf("remove incomplete dependency %s (backup: %s): %w", change.target, change.backup, err))
				continue
			}
		}
		if change.backup != "" {
			if err := state.rename(change.backup, change.target); err != nil {
				failures = append(failures, fmt.Errorf("restore dependency %s from %s: %w", change.target, change.backup, err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	return state.commit()
}

func (state *dependencyInstallState) commit() error {
	var failures []error
	for _, stage := range state.stages {
		if err := state.removeAll(stage); err != nil {
			failures = append(failures, fmt.Errorf("clean dependency staging directory %s: %w", stage, err))
		}
	}
	return errors.Join(failures...)
}

func ensureManagedDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create managed dependency directory %s: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect managed dependency directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("managed dependency directory %s must be a directory, not a symbolic link or file", path)
	}
	return nil
}

func validateManagedDependencyName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) || name == ".resources" || strings.HasPrefix(name, ".sampctl-") {
		return fmt.Errorf("invalid managed dependency directory name %q", name)
	}
	return nil
}

func copyDependencyForStaging(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return copydir.Copy(source, destination, copydir.Options{
		OnSymlink:  func(string) copydir.SymlinkAction { return copydir.Shallow },
		Skip:       func(_ os.FileInfo, _, _ string) (bool, error) { return false, ctx.Err() },
		WrapReader: func(reader io.Reader) io.Reader { return dependencyContextReader{ctx: ctx, reader: reader} },
	})
}

type dependencyContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader dependencyContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
