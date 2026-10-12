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

	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type projectFileSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

type dependencyProjectState struct {
	manifest  projectFileSnapshot
	lockfile  *projectFileSnapshot
	pkg       pawnpackage.Package
	resolved  PackageResolvedState
	lockState *lockfile.Lockfile
	lock      *flock.Flock
}

func snapshotProjectFile(path string) (projectFileSnapshot, error) {
	snapshot := projectFileSnapshot{path: path, mode: 0o644}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	if !info.Mode().IsRegular() {
		return snapshot, fmt.Errorf("%s must be a regular file", path)
	}
	snapshot.exists = true
	snapshot.mode = info.Mode().Perm()
	snapshot.data, err = os.ReadFile(path)
	return snapshot, err
}

func (snapshot projectFileSnapshot) restore() error {
	info, err := os.Lstat(snapshot.path)
	if errors.Is(err, os.ErrNotExist) && !snapshot.exists {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info != nil && !info.Mode().IsRegular() {
		return fmt.Errorf("cannot restore %s: path is no longer a regular file", snapshot.path)
	}
	if !snapshot.exists {
		return os.Remove(snapshot.path)
	}
	current, readErr := os.ReadFile(snapshot.path)
	if readErr == nil && bytes.Equal(current, snapshot.data) && info.Mode().Perm() == snapshot.mode {
		return nil
	}
	return fs.WriteFileAtomic(snapshot.path, snapshot.data, fs.PermDirPrivate, snapshot.mode)
}

func (pcx *PackageContext) beginDependencyOperation(ctx context.Context) (*dependencyProjectState, error) {
	return pcx.beginDependencyOperationWithLockfile(ctx, true)
}

func (pcx *PackageContext) beginDependencyOperationWithLockfile(ctx context.Context, includeLockfile bool) (*dependencyProjectState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(pcx.Package.LocalPath, ".sampctl-dependencies.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("project lock %s must be a regular file", lockPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	projectLock := flock.New(lockPath)
	locked, err := projectLock.TryLock()
	if err != nil {
		return nil, &OperationError{Operation: "project", Err: errors.Join(err, projectLock.Close())}
	}
	if !locked {
		return nil, &OperationError{Operation: "project", Err: errors.Join(errors.New("another sampctl dependency operation is using this project; retry when it finishes"), projectLock.Close())}
	}
	state, err := pcx.snapshotDependencyProject(includeLockfile)
	if err != nil {
		return nil, errors.Join(err, projectLock.Unlock())
	}
	state.lock = projectLock
	if resolver, ok := pcx.lockfileResolver.(*lockfile.Resolver); ok && includeLockfile {
		if err := resolver.Reload(); err != nil {
			return nil, errors.Join(&OperationError{Operation: "read lockfile", Err: err}, projectLock.Unlock())
		}
		data, err := json.Marshal(resolver.GetLockfile())
		if err != nil {
			return nil, errors.Join(err, projectLock.Unlock())
		}
		if err := json.Unmarshal(data, state.lockState); err != nil {
			return nil, errors.Join(err, projectLock.Unlock())
		}
		if state.lockfile.exists {
			if err := json.Unmarshal(state.lockfile.data, state.lockState); err != nil {
				return nil, errors.Join(err, projectLock.Unlock())
			}
		}
	}
	pcx.dependencyInstalls = newDependencyInstallState()
	return state, nil
}

func (pcx *PackageContext) snapshotDependencyProject(includeLockfile bool) (*dependencyProjectState, error) {
	path, _, _, err := pcx.readDefinitionSnapshot()
	if err != nil {
		return nil, &OperationError{Operation: "read manifest", Err: err}
	}
	manifest, err := snapshotProjectFile(path)
	if err != nil {
		return nil, &OperationError{Operation: "read manifest", Err: err}
	}
	if pcx.loadedDefinition != nil && !bytes.Equal(manifest.data, pcx.loadedDefinition) {
		return nil, &OperationError{Operation: "project", Err: errors.New("project configuration changed since loading; reload the project and retry")}
	}
	diskPkg, err := pawnpackage.PackageFromDefinition(manifest.data, pcx.Package.Format)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(diskPkg.Dependencies, pcx.Package.Dependencies) || !slices.Equal(diskPkg.Development, pcx.Package.Development) {
		return nil, errors.New("project dependencies changed since loading; reload the project and retry")
	}
	state := &dependencyProjectState{manifest: manifest, pkg: pcx.Package, resolved: pcx.PackageResolvedState}
	if includeLockfile {
		locked, err := snapshotProjectFile(filepath.Join(pcx.Package.LocalPath, lockfile.Filename))
		if err != nil {
			return nil, &OperationError{Operation: "read lockfile", Err: err}
		}
		state.lockfile = &locked
	}
	state.pkg.Dependencies = slices.Clone(pcx.Package.Dependencies)
	state.pkg.Development = slices.Clone(pcx.Package.Development)
	if lf := pcx.GetLockfile(); lf != nil {
		data, marshalErr := json.Marshal(lf)
		if marshalErr != nil {
			return nil, marshalErr
		}
		state.lockState = &lockfile.Lockfile{}
		if err := json.Unmarshal(data, state.lockState); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (pcx *PackageContext) finishDependencyOperation(state *dependencyProjectState, operationErr error) error {
	installs := pcx.dependencyInstalls
	pcx.dependencyInstalls = nil
	var savedDefinition []byte
	if operationErr == nil && pcx.loadedDefinition != nil {
		var err error
		savedDefinition, err = os.ReadFile(state.manifest.path)
		if err != nil {
			operationErr = &OperationError{Operation: "manifest", Err: err}
		}
	}
	if operationErr != nil {
		rollbackErr := errors.Join(installs.rollback(), state.manifest.restore())
		if state.lockfile != nil {
			rollbackErr = errors.Join(rollbackErr, state.lockfile.restore())
		}
		pcx.Package = state.pkg
		pcx.PackageResolvedState = state.resolved
		if pcx.loadedDefinition != nil {
			pcx.loadedDefinition = slices.Clone(state.manifest.data)
		}
		if resolver, ok := pcx.lockfileResolver.(*lockfile.Resolver); ok {
			resolver.Restore(state.lockState)
		} else if lf := pcx.GetLockfile(); lf != nil && state.lockState != nil {
			*lf = *state.lockState
		}
		if rollbackErr != nil {
			operationErr = errors.Join(operationErr, &OperationError{Operation: "rollback", Err: rollbackErr})
		}
	} else {
		if pcx.loadedDefinition != nil {
			pcx.loadedDefinition = savedDefinition
		}
		if err := installs.commit(); err != nil {
			operationErr = &OperationError{Operation: "cleanup", Err: err}
		}
	}
	if err := state.lock.Unlock(); err != nil {
		operationErr = errors.Join(operationErr, &OperationError{Operation: "cleanup", Err: err})
	}
	return operationErr
}

func (pcx *PackageContext) syncDependencyLockfile(ctx context.Context) error {
	if !pcx.HasLockfileResolver() {
		return nil
	}
	deps, err := pcx.lockfileDependencies(ctx, DependencyUpdateRequest{}, false)
	if err != nil {
		return err
	}
	if err := pcx.recordRootLocalDependencies(); err != nil {
		return err
	}
	pcx.pruneLockfileDependencies(lockfileDependencyMetas(deps))
	pcx.updateLockfileRelationships(deps)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := pcx.PackageLockfileState.SaveLockfile(); err != nil {
		return &OperationError{Operation: "lockfile", Err: err}
	}
	return ctx.Err()
}

func (pcx *PackageContext) updateLockfileRelationships(deps []lockfileDependencyState) {
	for _, dep := range deps {
		lf := pcx.GetLockfile()
		if lf == nil {
			continue
		}
		key := lockfile.DependencyKey(dep.Meta)
		entry, ok := lf.Dependencies[key]
		if !ok {
			continue
		}
		entry.Transitive = !dep.Direct
		entry.RequiredBy = nil
		for parent := range dep.RequiredBy {
			entry.RequiredBy = append(entry.RequiredBy, parent)
		}
		slices.Sort(entry.RequiredBy)
		lf.Dependencies[key] = entry
	}
}
