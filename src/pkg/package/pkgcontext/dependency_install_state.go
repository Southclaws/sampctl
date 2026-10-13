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

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
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
	journal       *dependencyJournal
}

func newDependencyInstallState() *dependencyInstallState {
	return &dependencyInstallState{rename: os.Rename, removeAll: os.RemoveAll}
}

func (state *dependencyInstallState) stage(vendor string, names ...string) (string, error) {
	vendor, err := filepath.Abs(vendor)
	if err != nil {
		return "", err
	}
	if err := ensureManagedDirectory(vendor); err != nil {
		return "", err
	}
	if state.journal != nil {
		path := filepath.Join(vendor, fmt.Sprintf(".sampctl-stage-%s-%d", state.journal.ID, len(state.journal.Stages)))
		relative, err := state.journal.relative(path)
		if err != nil {
			return "", err
		}
		allowed := append([]string{".resources"}, names...)
		for _, name := range names {
			allowed = append(allowed, "."+name+".sampctl.lock")
		}
		state.journal.Stages = append(state.journal.Stages, dependencyRecoveryStage{Path: relative, Allowed: allowed})
		if err := state.journal.validate(); err != nil {
			return "", err
		}
		if err := state.journal.save(); err != nil {
			return "", err
		}
		state.stages = append(state.stages, path)
		if err := os.Mkdir(path, 0o700); err != nil {
			return "", fmt.Errorf("create dependency staging directory: %w", err)
		}
		if err := fs.WriteFileAtomic(filepath.Join(path, ".sampctl-owner"), []byte(state.journal.ID), 0o700, 0o600); err != nil {
			return "", err
		}
		return path, fs.SyncDir(vendor)
	}
	path, err := os.MkdirTemp(vendor, ".sampctl-stage-")
	if err != nil {
		return "", fmt.Errorf("create dependency staging directory: %w", err)
	}
	state.stages = append(state.stages, path)
	return path, nil
}

func (state *dependencyInstallState) promote(source, target string) error {
	return state.promoteContext(context.Background(), source, target)
}

func (state *dependencyInstallState) promoteContext(ctx context.Context, source, target string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
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
	if state.journal != nil {
		return state.promoteRecorded(ctx, source, target, stage, info)
	}
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("dependency destination %s must be a directory, not a symbolic link or file", target)
		}
		backup, err := os.MkdirTemp(filepath.Dir(source), ".previous-")
		if err != nil {
			return fmt.Errorf("prepare dependency backup: %w", err)
		}
		change.backup = filepath.Join(backup, "dependency")
		state.installations = append(state.installations, change)
		if err := state.rename(target, change.backup); err != nil {
			state.installations = state.installations[:len(state.installations)-1]
			return fmt.Errorf("back up dependency %s: %w", target, err)
		}
	} else {
		state.installations = append(state.installations, change)
	}
	if err := state.rename(source, target); err != nil {
		return fmt.Errorf("promote dependency %s: %w", target, err)
	}
	state.installations[len(state.installations)-1].promoted = true
	return nil
}

func (state *dependencyInstallState) allowStageSource(source string) error {
	if state.journal == nil {
		return nil
	}
	stage, err := state.stageForSource(source)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(stage, source)
	if err != nil {
		return err
	}
	name := strings.Split(relative, string(filepath.Separator))[0]
	for i := range state.journal.Stages {
		if state.journal.path(state.journal.Stages[i].Path) == stage {
			for _, allowed := range state.journal.Stages[i].Allowed {
				if allowed == name {
					return nil
				}
			}
			state.journal.Stages[i].Allowed = append(state.journal.Stages[i].Allowed, name)
			return state.journal.save()
		}
	}
	return fmt.Errorf("unrecorded staging source %s", source)
}

func (state *dependencyInstallState) promoteRecorded(ctx context.Context, source, target, stage string, previous os.FileInfo) error {
	if previous != nil && (!previous.IsDir() || previous.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("dependency destination %s must be a directory", target)
	}
	if err := state.allowStageSource(source); err != nil {
		return err
	}
	newHash, err := recoveryTreeHash(ctx, source, true)
	if err != nil || newHash == "" {
		return errors.Join(err, errors.New("staged dependency is missing"))
	}
	oldHash, err := recoveryTreeHash(ctx, target, false)
	if err != nil {
		return err
	}
	sourceRel, err := state.journal.relative(source)
	if err != nil {
		return err
	}
	targetRel, err := state.journal.relative(target)
	if err != nil {
		return err
	}
	for _, change := range state.journal.Changes {
		if change.Target == targetRel && change.NewHash == newHash && oldHash == newHash {
			return nil
		}
	}
	change := dependencyRecoveryChange{Source: sourceRel, Target: targetRel, OldHash: oldHash, NewHash: newHash, Phase: "prepared"}
	if oldHash != "" {
		backup := filepath.Join(stage, fmt.Sprintf(".previous-%d", len(state.journal.Changes)))
		change.Backup, err = state.journal.relative(backup)
		if err != nil {
			return err
		}
		if err := state.allowStageSource(backup); err != nil {
			return err
		}
	}
	state.journal.Changes = append(state.journal.Changes, change)
	if err := state.journal.validate(); err != nil {
		state.journal.Changes = state.journal.Changes[:len(state.journal.Changes)-1]
		return err
	}
	if err := state.journal.save(); err != nil {
		return err
	}
	installation := dependencyInstallation{target: target}
	if change.Backup != "" {
		installation.backup = state.journal.path(change.Backup)
	}
	state.installations = append(state.installations, installation)
	if err := ctx.Err(); err != nil {
		return err
	}
	if change.Backup != "" {
		if err := renameRecoveryPath(state.rename, target, installation.backup); err != nil {
			return fmt.Errorf("back up dependency %s: %w", target, err)
		}
		state.journal.Changes[len(state.journal.Changes)-1].Phase = "backed_up"
		if err := state.journal.save(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := renameRecoveryPath(state.rename, source, target); err != nil {
		return fmt.Errorf("promote dependency %s: %w", target, err)
	}
	state.installations[len(state.installations)-1].promoted = true
	state.journal.Changes[len(state.journal.Changes)-1].Phase = "promoted"
	return state.journal.save()
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
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
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
	if state.journal != nil {
		return state.journal.recover(context.Background(), state.rename, state.removeAll)
	}
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
	if state.journal != nil {
		return state.journal.recover(context.Background(), state.rename, state.removeAll)
	}
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
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) || strings.EqualFold(name, ".resources") || strings.HasPrefix(strings.ToLower(name), ".sampctl-") {
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
