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
	journal   *dependencyJournal
	recovered bool
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
	if snapshot.path == "" {
		return nil
	}
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

func (pcx *PackageContext) beginDependencyOperation(ctx context.Context, operation ...string) (*dependencyProjectState, error) {
	return pcx.beginDependencyOperationWithLockfile(ctx, true, operation...)
}

func (pcx *PackageContext) beginDependencyOperationWithLockfile(ctx context.Context, includeLockfile bool, operation ...string) (*dependencyProjectState, error) {
	return pcx.beginDependencyOperationState(ctx, includeLockfile, false, operation...)
}

func (pcx *PackageContext) beginDependencyOperationState(ctx context.Context, includeLockfile, allowMissingManifest bool, operation ...string) (*dependencyProjectState, error) {
	projectLock, err := lockDependencyProject(ctx, pcx.Package.LocalPath)
	if err != nil {
		return nil, err
	}
	recovery, err := recoverDependencyOperationLocked(ctx, pcx.Package.LocalPath)
	if err != nil {
		return nil, errors.Join(err, projectLock.Unlock())
	}
	if recovery.Pending && pcx.Package.Format != "" {
		previous := pcx.Package
		pkg, err := pcx.readPackageDefinition(previous.LocalPath)
		if err != nil {
			return nil, errors.Join(err, projectLock.Unlock())
		}
		pkg.LocalPath, pkg.Vendor, pkg.Parent = previous.LocalPath, previous.Vendor, previous.Parent
		pkg.Tag = previous.Tag
		pcx.Package = pkg
		pcx.PackageResolvedState = PackageResolvedState{}
	}
	state, err := pcx.snapshotDependencyProject(includeLockfile)
	if err != nil && allowMissingManifest && (errors.Is(err, os.ErrNotExist) || pcx.Package.Format == "") {
		state = &dependencyProjectState{pkg: pcx.Package, resolved: pcx.PackageResolvedState}
		state.pkg.Dependencies = slices.Clone(pcx.Package.Dependencies)
		state.pkg.Development = slices.Clone(pcx.Package.Development)
		if includeLockfile {
			file, snapshotErr := snapshotProjectFile(filepath.Join(pcx.Package.LocalPath, lockfile.Filename))
			if snapshotErr != nil {
				return nil, errors.Join(snapshotErr, projectLock.Unlock())
			}
			state.lockfile = &file
		}
		err = nil
	}
	if err != nil {
		return nil, errors.Join(err, projectLock.Unlock())
	}
	state.lock = projectLock
	state.recovered = recovery.Pending
	if resolver, ok := pcx.lockfileResolver.(*lockfile.Resolver); ok && includeLockfile {
		if err := resolver.Reload(); err != nil {
			return nil, errors.Join(&OperationError{Operation: "read lockfile", Err: err}, projectLock.Unlock())
		}
		data, err := json.Marshal(resolver.GetLockfile())
		if err != nil {
			return nil, errors.Join(err, projectLock.Unlock())
		}
		state.lockState = &lockfile.Lockfile{}
		if err := json.Unmarshal(data, state.lockState); err != nil {
			return nil, errors.Join(err, projectLock.Unlock())
		}
		if state.lockfile.exists {
			if err := json.Unmarshal(state.lockfile.data, state.lockState); err != nil {
				return nil, errors.Join(err, projectLock.Unlock())
			}
		}
	}
	files := []projectFileSnapshot{state.manifest}
	if state.lockfile != nil {
		files = append(files, *state.lockfile)
	}
	name := "dependency operation"
	if len(operation) != 0 {
		name = operation[0]
	}
	state.journal, err = newDependencyJournal(pcx.Package.LocalPath, name, files...)
	if err != nil {
		return nil, errors.Join(err, projectLock.Unlock())
	}
	pcx.dependencyInstalls = newDependencyInstallState()
	pcx.dependencyInstalls.journal = state.journal
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
		return nil, &OperationError{Operation: "project", Err: errors.New("project configuration changed since loading. Reload the project and retry")}
	}
	diskPkg, err := pawnpackage.PackageFromDefinition(manifest.data, pcx.Package.Format)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(diskPkg.Dependencies, pcx.Package.Dependencies) || !slices.Equal(diskPkg.Development, pcx.Package.Development) {
		return nil, errors.New("project dependencies changed since loading. Reload the project and retry")
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
	var commitErr error
	if operationErr == nil && pcx.loadedDefinition != nil && state.manifest.path != "" {
		var err error
		savedDefinition, err = os.ReadFile(state.manifest.path)
		if err != nil {
			operationErr = &OperationError{Operation: "manifest", Err: err}
		}
	}
	if operationErr == nil {
		if err := state.journal.validateState(context.Background()); err != nil {
			operationErr = &OperationError{Operation: "commit", Err: err}
		} else {
			state.journal.Phase = "committed"
			if err := state.journal.save(); err != nil {
				// Atomic persistence may fail after the rename during directory sync.
				// Keep the on-disk state authoritative and retain recovery evidence.
				disk, readErr := readDependencyJournal(state.journal.project)
				if readErr == nil && disk != nil && disk.Phase == "committed" {
					commitErr = &OperationError{Operation: "cleanup", Err: err}
				} else if readErr == nil && disk != nil && disk.Phase == "active" {
					state.journal.Phase = "active"
					operationErr = &OperationError{Operation: "commit", Err: err}
				} else {
					return &OperationError{Operation: "dependency operation", Outcome: OutcomeRollbackIncomplete, Err: errors.Join(err, readErr, state.lock.Unlock())}
				}
			}
		}
	}
	outcome := OutcomeCommitted
	if operationErr != nil {
		rollbackErr := installs.rollback()
		outcome = OutcomeRolledBack
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
			if state.journal.Phase == "restored" && state.journal.validateState(context.Background()) == nil {
				operationErr = errors.Join(operationErr, &OperationError{Operation: "cleanup", Err: rollbackErr})
			} else {
				outcome = OutcomeRollbackIncomplete
				operationErr = errors.Join(operationErr, &OperationError{Operation: "rollback", Err: rollbackErr})
			}
		}
	} else {
		if pcx.loadedDefinition != nil {
			pcx.loadedDefinition = savedDefinition
		}
		if err := installs.commit(); err != nil {
			commitErr = errors.Join(commitErr, &OperationError{Operation: "cleanup", Err: err})
		}
		operationErr = commitErr
	}
	if err := state.lock.Unlock(); err != nil {
		operationErr = errors.Join(operationErr, &OperationError{Operation: "cleanup", Err: err})
	}
	if operationErr != nil {
		name := "dependency operation"
		var step *OperationError
		if errors.As(operationErr, &step) {
			name = step.Operation
		}
		return &OperationError{Operation: name, Outcome: outcome, Err: operationErr}
	}
	return operationErr
}

func (pcx *PackageContext) writeDependencyDefinition() error {
	data, err := pcx.Package.EncodeDefinition()
	if err != nil {
		return err
	}
	if pcx.dependencyInstalls != nil && pcx.dependencyInstalls.journal != nil {
		path := filepath.Join(pcx.Package.LocalPath, "pawn."+pcx.Package.Format)
		if err := pcx.dependencyInstalls.journal.recordWrite(path, data); err != nil {
			return err
		}
	}
	return pcx.Package.WriteDefinition()
}

func (pcx *PackageContext) recordDependencyLockfileWrite(lf *lockfile.Lockfile) error {
	if lf == nil || pcx.dependencyInstalls == nil || pcx.dependencyInstalls.journal == nil {
		return nil
	}
	data, err := json.Marshal(lf)
	if err != nil {
		return err
	}
	return pcx.dependencyInstalls.journal.recordWrite(filepath.Join(pcx.Package.LocalPath, lockfile.Filename), data)
}

func (pcx *PackageContext) saveDependencyLockfile() error {
	if err := pcx.recordDependencyLockfileWrite(pcx.GetLockfile()); err != nil {
		return err
	}
	return pcx.PackageLockfileState.SaveLockfile()
}

func dependencyOperationCommitted(err error) bool {
	var operation *OperationError
	return errors.As(err, &operation) && operation.Outcome == OutcomeCommitted
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
	if err := pcx.saveDependencyLockfile(); err != nil {
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
