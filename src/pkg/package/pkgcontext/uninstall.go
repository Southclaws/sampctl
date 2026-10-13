package pkgcontext

import (
	"context"
	"slices"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

// Uninstall removes dependency declarations and synchronizes an existing lockfile.
func (pcx *PackageContext) Uninstall(
	targets []versioning.DependencyString,
	development bool,
) (err error) {
	state, err := pcx.beginDependencyOperation(context.Background(), "uninstall")
	if err != nil {
		return err
	}
	defer func() { err = pcx.finishDependencyOperation(state, err) }()
	for _, target := range targets {
		err = pcx.uninstall(target, development)
		if err != nil {
			return
		}
	}
	if slices.Equal(state.pkg.Dependencies, pcx.Package.Dependencies) && slices.Equal(state.pkg.Development, pcx.Package.Development) {
		return nil
	}
	lf, err := lockfile.Load(pcx.Package.LocalPath)
	if err != nil {
		return &OperationError{Operation: "read lockfile", Err: err}
	}
	if lf != nil {
		if err := pcx.pruneUninstalledLockfile(lf); err != nil {
			return err
		}
	}
	if err := pcx.writeDependencyDefinition(); err != nil {
		return &OperationError{Operation: "manifest", Err: err}
	}
	if lf != nil {
		if err := pcx.recordDependencyLockfileWrite(lf); err != nil {
			return &OperationError{Operation: "lockfile", Err: err}
		}
		if err := lockfile.Save(pcx.Package.LocalPath, lf); err != nil {
			return &OperationError{Operation: "lockfile", Err: err}
		}
		if current := pcx.GetLockfile(); current != nil {
			*current = *lf
		}
	}
	return nil
}

func (pcx *PackageContext) pruneUninstalledLockfile(lf *lockfile.Lockfile) error {
	direct := make(map[string]bool)
	for _, dep := range pcx.Package.GetAllDependencies() {
		meta, err := dep.Explode()
		if err != nil {
			return &DependencyError{Dependency: string(dep), Operation: "parse", Err: err}
		}
		direct[lockfile.DependencyKey(normalizeLockfileDependency(meta))] = true
	}
	required := make(map[string]bool)
	for key := range direct {
		required[key] = true
	}
	// Older lockfiles may omit relationship data. Keep such transitive entries
	// while direct dependencies remain, rather than guessing they are unused.
	for key, entry := range lf.Dependencies {
		if entry.Transitive && len(entry.RequiredBy) == 0 && len(direct) > 0 {
			required[key] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for key, entry := range lf.Dependencies {
			if required[key] {
				continue
			}
			for _, parent := range entry.RequiredBy {
				if required[parent] {
					required[key] = true
					changed = true
					break
				}
			}
		}
	}
	for key, entry := range lf.Dependencies {
		if !required[key] {
			delete(lf.Dependencies, key)
			continue
		}
		entry.Transitive = !direct[key]
		parents := slices.Clone(entry.RequiredBy)
		entry.RequiredBy = nil
		for _, parent := range parents {
			if required[parent] {
				entry.RequiredBy = append(entry.RequiredBy, parent)
			}
		}
		lf.Dependencies[key] = entry
	}
	return nil
}

func (pcx *PackageContext) uninstall(
	target versioning.DependencyString,
	development bool,
) (err error) {
	_, err = target.Explode()
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
	}

	if development {
		err = pcx.uninstallDev(target)
	} else {
		err = pcx.uninstallDep(target)
	}
	return
}

// regular dependencies
func (pcx *PackageContext) uninstallDep(target versioning.DependencyString) (err error) {
	idx := -1
	for i, dep := range pcx.Package.Dependencies {
		if matchesUninstallTarget(target, dep) {
			idx = i
			break
		}
	}

	if idx == -1 {
		print.Warn("target does not exist in dependencies")
		return
	}

	pcx.Package.Dependencies = append(pcx.Package.Dependencies[:idx], pcx.Package.Dependencies[idx+1:]...)

	return
}

// development dependencies
func (pcx *PackageContext) uninstallDev(target versioning.DependencyString) (err error) {
	idx := -1
	for i, dep := range pcx.Package.Development {
		if matchesUninstallTarget(target, dep) {
			idx = i
			break
		}
	}

	if idx == -1 {
		print.Warn("target does not exist in dependencies")
		return
	}

	pcx.Package.Development = append(pcx.Package.Development[:idx], pcx.Package.Development[idx+1:]...)

	return
}

func matchesUninstallTarget(target, dependency versioning.DependencyString) bool {
	targetMeta, err := target.Explode()
	if err != nil {
		return false
	}
	dependencyMeta, err := dependency.Explode()
	if err != nil {
		return false
	}
	return (DependencyUpdateRequest{Target: string(target), TargetMeta: targetMeta}).Matches(dependencyMeta)
}
