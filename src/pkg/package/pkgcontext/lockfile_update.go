package pkgcontext

import (
	"context"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type lockfileDependencyState struct {
	Meta       versioning.DependencyMeta
	Direct     bool
	RequiredBy map[string]struct{}
}

// UpdateLockfile refreshes lockfile dependency entries without installing dependencies into the working tree.
func (pcx *PackageContext) UpdateLockfile(ctx context.Context, forceUpdate bool) error {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return nil
	}

	if err := pcx.EnsureDependenciesCached(); err != nil {
		return errors.Wrap(err, "failed to refresh dependency cache")
	}

	deps, err := pcx.currentLockfileDependencies()
	if err != nil {
		return err
	}

	for _, dep := range deps {
		meta := dep.Meta
		if meta.IsLocalScheme() {
			if err := pcx.PackageLockfileState.RecordLocalDependency(meta); err != nil {
				return errors.Wrap(err, "failed to record local dependency")
			}
			continue
		}

		resolvedMeta, err := pcx.resolveLockfileDependencyMeta(ctx, meta, forceUpdate)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve dependency %s", meta)
		}

		repo, err := pcx.EnsureDependencyCached(resolvedMeta, forceUpdate)
		if err != nil {
			return errors.Wrapf(err, "failed to ensure cached dependency %s", resolvedMeta)
		}

		resolution, err := resolveDependencyLock(resolvedMeta, repo)
		if err != nil {
			return errors.Wrapf(err, "failed to resolve lockfile state for %s", resolvedMeta)
		}

		requiredBy := firstRequiredBy(dep.RequiredBy)
		if err := pcx.PackageLockfileState.RecordDependencyResolution(meta, resolution, !dep.Direct, requiredBy); err != nil {
			return errors.Wrapf(err, "failed to record lockfile resolution for %s", meta)
		}
	}

	pcx.pruneLockfileDependencies(lockfileDependencyMetas(deps))
	print.Verb("lockfile dependency metadata refreshed from cache")

	return nil
}

func (pcx *PackageContext) resolveLockfileDependencyMeta(
	ctx context.Context,
	meta versioning.DependencyMeta,
	forceUpdate bool,
) (versioning.DependencyMeta, error) {
	resolvedMeta := pcx.PackageLockfileState.LockedVersion(meta, forceUpdate)
	resolvedMeta, err := pcx.resolveDynamicDependencyReference(ctx, resolvedMeta, meta, forceUpdate)
	if err != nil {
		return versioning.DependencyMeta{}, err
	}

	if resolvedMeta.Tag == "" && resolvedMeta.Branch == "" && resolvedMeta.Commit == "" {
		tag, err := pcx.resolveLatestTag(ctx, resolvedMeta, forceUpdate)
		if err == nil && tag != "" {
			resolvedMeta.Tag = tag
		}
	}

	return resolvedMeta, nil
}

func (pcx *PackageContext) currentLockfileDependencies() ([]lockfileDependencyState, error) {
	states := make(map[string]*lockfileDependencyState)
	visited := make(map[string]bool)

	var walk func(deps []versioning.DependencyString, parent string, direct bool) error
	walk = func(deps []versioning.DependencyString, parent string, direct bool) error {
		for _, depStr := range deps {
			meta, err := depStr.Explode()
			if err != nil {
				return errors.Wrapf(err, "failed to parse dependency string %s", depStr)
			}

			normalized := normalizeLockfileDependency(meta)
			key := lockfile.DependencyKey(normalized)
			state, ok := states[key]
			if !ok {
				state = &lockfileDependencyState{Meta: normalized, RequiredBy: make(map[string]struct{})}
				states[key] = state
			}
			if direct {
				state.Direct = true
			}
			if !direct && parent != "" {
				state.RequiredBy[parent] = struct{}{}
			}

			if normalized.IsLocalScheme() || visited[key] {
				continue
			}
			visited[key] = true

			pkg, err := pcx.lockfileDependencyPackage(normalized)
			if err != nil {
				return err
			}
			if pkg.Format == "" {
				continue
			}

			nextDeps := pkg.Dependencies
			if pkg.Parent {
				nextDeps = pkg.GetAllDependencies()
			}
			if err := walk(nextDeps, key, false); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(pcx.Package.GetAllDependencies(), "", true); err != nil {
		return nil, err
	}

	deps := make([]lockfileDependencyState, 0, len(states))
	for _, state := range states {
		deps = append(deps, *state)
	}

	return deps, nil
}

func (pcx *PackageContext) lockfileDependencyPackage(meta versioning.DependencyMeta) (pawnpackage.Package, error) {
	pkg, err := pawnpackage.GetCachedPackage(meta, pcx.CacheDir)
	if err == nil && pkg.Format != "" {
		return pkg, nil
	}

	pkg, err = pawnpackage.PackageFromDir(meta.CachePath(pcx.CacheDir))
	if err != nil {
		return pawnpackage.Package{}, errors.Wrapf(err, "failed to load cached package definition for %s", meta)
	}

	return pkg, nil
}

func normalizeLockfileDependency(meta versioning.DependencyMeta) versioning.DependencyMeta {
	if !meta.IsURLScheme() || meta.IsLocalScheme() {
		return meta
	}

	return versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
		SSH:    meta.SSH,
	}
}

func (pcx *PackageContext) recordRootLocalDependencies() {
	for _, depStr := range pcx.Package.GetAllDependencies() {
		meta, err := depStr.Explode()
		if err != nil || !meta.IsLocalScheme() {
			continue
		}
		if err := pcx.PackageLockfileState.RecordLocalDependency(meta); err != nil {
			print.Warn("failed to record local dependency in lockfile:", err)
		}
	}
}

func (pcx *PackageContext) pruneLockfileDependencies(current []versioning.DependencyMeta) {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return
	}
	pcx.PackageLockfileState.PruneMissingDependencies(current)
}

func firstRequiredBy(requiredBy map[string]struct{}) string {
	for key := range requiredBy {
		return key
	}
	return ""
}

func lockfileDependencyMetas(states []lockfileDependencyState) []versioning.DependencyMeta {
	deps := make([]versioning.DependencyMeta, 0, len(states))
	for _, state := range states {
		deps = append(deps, state.Meta)
	}
	return deps
}