package pkgcontext

import (
	"context"
	stderrors "errors"

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
	resolution lockfile.DependencyResolution
}

// UpdateLockfile refreshes lockfile dependency entries without installing dependencies into the working tree.
func (pcx *PackageContext) UpdateLockfile(ctx context.Context, request DependencyUpdateRequest) error {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return nil
	}

	deps, err := pcx.lockfileDependencies(ctx, request, true)
	if err != nil {
		return err
	}

	for _, dep := range deps {
		var err error
		if dep.Meta.IsLocalScheme() {
			err = pcx.RecordLocalDependency(dep.Meta)
		} else {
			err = pcx.RecordDependencyResolution(dep.Meta, dep.resolution, !dep.Direct, "")
		}
		if err != nil {
			return &DependencyError{Dependency: dep.Meta.String(), Operation: "record lockfile", Err: err}
		}
	}

	pcx.pruneLockfileDependencies(lockfileDependencyMetas(deps))
	pcx.updateLockfileRelationships(deps)
	print.Verb("lockfile dependency metadata refreshed from cache")

	return nil
}

func (pcx *PackageContext) resolveLockfileDependencyMeta(
	ctx context.Context,
	meta versioning.DependencyMeta,
	forceUpdate, upgradePin, staleOverrideLock bool,
) (versioning.DependencyMeta, error) {
	resolvedMeta := pcx.PackageLockfileState.LockedVersion(meta, forceUpdate || staleOverrideLock)
	resolvedMeta, err := pcx.resolveDynamicDependencyReference(ctx, resolvedMeta, meta, forceUpdate)
	if err != nil {
		return versioning.DependencyMeta{}, err
	}

	if upgradePin && isPinnedTagDependency(meta) {
		tag, err := pcx.resolveLatestTag(ctx, meta, true)
		if err != nil {
			if isMissingLatestReleaseError(err) {
				print.Warn(meta, "has no tags or releases. Keeping the lockfile pin")
				return resolvedMeta, nil
			}

			return versioning.DependencyMeta{}, err
		}
		if tag != "" {
			resolvedMeta.Tag = tag
		}
	}

	if resolvedMeta.Tag == "" && resolvedMeta.Branch == "" && resolvedMeta.Commit == "" {
		tag, err := pcx.resolveLatestTag(ctx, resolvedMeta, forceUpdate)
		if err != nil && !isMissingLatestReleaseError(err) {
			return versioning.DependencyMeta{}, err
		}
		if tag != "" {
			resolvedMeta.Tag = tag
		}
	}

	return resolvedMeta, nil
}

func (pcx *PackageContext) lockfileDependencies(ctx context.Context, request DependencyUpdateRequest, refresh bool) ([]lockfileDependencyState, error) {
	policy, err := versioning.NewProjectOverrides(pcx.Package.Overrides, pcx.Package.Format)
	if err != nil {
		return nil, err
	}
	selection := request.selectDependencies(policy, pcx.Package.GetAllDependencies())
	activePins := make(map[versioning.DependencyMeta]bool)
	for {
		deps, retry, err := pcx.walkLockfileDependencies(ctx, selection, policy, refresh, activePins)
		if !retry || ctx.Err() != nil {
			return deps, err
		}
	}
}

// A newer release can introduce an override for a dependency already visited.
// Rewalk with the newly discovered pin before recording any lockfile entries.
func (pcx *PackageContext) walkLockfileDependencies(ctx context.Context, selection dependencyUpdateSelection, policy versioning.ProjectOverrides, refresh bool, activePins map[versioning.DependencyMeta]bool) ([]lockfileDependencyState, bool, error) {
	retry := false
	upgrades := make(map[versioning.DependencyMeta]bool)
	states := make(map[string]*lockfileDependencyState)
	visited := make(map[string]bool)
	directDependencies := make(map[string]bool)
	directConstraints := pcx.directDependencyConstraints(policy)
	destinations := make(map[string]versioning.DependencyMeta)
	for _, declaration := range pcx.Package.GetAllDependencies() {
		if meta, err := policy.Resolve(declaration); err == nil {
			directDependencies[lockfile.DependencyKey(normalizeLockfileDependency(meta))] = true
		}
	}

	var walk func(deps []versioning.DependencyString, parent string, direct bool) error
	walk = func(deps []versioning.DependencyString, parent string, direct bool) error {
		var failures []error
		for _, depStr := range deps {
			if err := ctx.Err(); err != nil {
				return stderrors.Join(append(failures, err)...)
			}
			meta, err := policy.Resolve(depStr)
			if err != nil {
				failures = append(failures, &DependencyError{Dependency: string(depStr), Operation: "parse", Err: err})
				continue
			}

			normalized := normalizeLockfileDependency(inheritDirectDependencyConstraint(meta, directConstraints))
			if !normalized.IsLocalScheme() {
				if err := registerDependencyDestination(destinations, normalized); err != nil {
					failures = append(failures, err)
					continue
				}
			}
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

			identity := managedDependencyIdentity(normalized)
			if refresh && isPinnedTagDependency(meta) && policy.Matches(depStr) && !activePins[identity] {
				activePins[identity] = true
				_, stale := pcx.GetLockfile().ConflictingOverrideTag(normalized)
				retry = retry || upgrades[identity] || stale && visited[key]
			}
			if visited[key] {
				continue
			}
			visited[key] = true
			if normalized.IsLocalScheme() {
				continue
			}

			var pkg pawnpackage.Package
			if refresh {
				rootDependency := directDependencies[key]
				force := selection.ShouldForceDependency(normalized, rootDependency)
				upgradePin := force && isPinnedTagDependency(normalized) && !activePins[identity]
				upgrades[identity] = upgradePin
				_, stale := pcx.GetLockfile().ConflictingOverrideTag(normalized)
				pkg, state.resolution, err = pcx.refreshLockfileDependency(ctx, normalized, force, upgradePin, activePins[identity] && stale)
			} else {
				pkg, err = pcx.lockfileDependencyPackageContext(ctx, normalized)
			}
			if err != nil {
				failures = append(failures, &DependencyError{Dependency: string(depStr), Operation: "resolve lockfile", Err: err})
				if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
					return stderrors.Join(failures...)
				}
				continue
			}
			if pkg.Format == "" {
				continue
			}

			if err := walk(pkg.Dependencies, key, false); err != nil {
				failures = append(failures, err)
				if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
					return stderrors.Join(failures...)
				}
			}
		}

		return stderrors.Join(failures...)
	}

	if err := walk(pcx.Package.GetAllDependencies(), "", true); err != nil {
		return nil, retry, err
	}

	deps := make([]lockfileDependencyState, 0, len(states))
	for _, state := range states {
		deps = append(deps, *state)
	}

	return deps, retry, nil
}

func (pcx *PackageContext) refreshLockfileDependency(ctx context.Context, meta versioning.DependencyMeta, force, upgradePin, staleOverrideLock bool) (pawnpackage.Package, lockfile.DependencyResolution, error) {
	resolved, err := pcx.resolveLockfileDependencyMeta(ctx, meta, force, upgradePin, staleOverrideLock)
	if err != nil {
		return pawnpackage.Package{}, lockfile.DependencyResolution{}, err
	}

	refreshCache := force
	if refreshCache {
		noOrigin, err := pcx.cachedRepoHasNoOriginContext(ctx, resolved)
		if err != nil {
			return pawnpackage.Package{}, lockfile.DependencyResolution{}, err
		}
		refreshCache = !noOrigin
	}
	if _, err := pcx.EnsureDependencyCachedContext(ctx, resolved, refreshCache); err != nil {
		return pawnpackage.Package{}, lockfile.DependencyResolution{}, err
	}

	path := resolved.CachePath(pcx.CacheDir)
	unlock, err := lockRepositoryPath(ctx, path)
	if err != nil {
		return pawnpackage.Package{}, lockfile.DependencyResolution{}, err
	}

	repo, err := pcx.repositoryStore().Open(path)
	var resolution lockfile.DependencyResolution
	if err == nil {
		resolution, err = resolveCachedDependencyLock(resolved, repo)
	}
	err = stderrors.Join(err, unlock())
	if err != nil {
		return pawnpackage.Package{}, lockfile.DependencyResolution{}, err
	}

	if previous, ok := pcx.PreviousDependency(meta); ok && resolved.Commit != "" && previous.Commit == resolution.Commit && previous.Resolved != "" {
		resolution.Resolved = previous.Resolved
	}
	resolved.Tag, resolved.Branch, resolved.Commit = "", "", resolution.Commit
	pkg, err := pcx.packageFromCachedRevisionContext(ctx, resolved, path)
	return pkg, resolution, err
}

func (pcx *PackageContext) lockfileDependencyPackageContext(ctx context.Context, meta versioning.DependencyMeta) (pawnpackage.Package, error) {
	meta = pcx.PackageLockfileState.LockedVersion(meta, false)
	pkg, err := pcx.packageFromCachedRevisionContext(ctx, meta, meta.CachePath(pcx.CacheDir))
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

func (pcx *PackageContext) recordRootLocalDependencies() error {
	policy, err := versioning.NewProjectOverrides(pcx.Package.Overrides, pcx.Package.Format)
	if err != nil {
		return err
	}
	for _, depStr := range pcx.Package.GetAllDependencies() {
		meta, err := policy.Resolve(depStr)
		if err != nil || !meta.IsLocalScheme() {
			continue
		}
		if err := pcx.PackageLockfileState.RecordLocalDependency(meta); err != nil {
			return &DependencyError{Dependency: string(depStr), Operation: "record lockfile", Err: err}
		}
	}

	return nil
}

func (pcx *PackageContext) pruneLockfileDependencies(current []versioning.DependencyMeta) {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return
	}
	pcx.PackageLockfileState.PruneMissingDependencies(current)
}

func lockfileDependencyMetas(states []lockfileDependencyState) []versioning.DependencyMeta {
	deps := make([]versioning.DependencyMeta, 0, len(states))
	for _, state := range states {
		deps = append(deps, state.Meta)
	}
	return deps
}

// UpdateProjectLockfile updates and saves dependency pins without installing files.
func (pcx *PackageContext) UpdateProjectLockfile(ctx context.Context, request DependencyUpdateRequest) (err error) {
	state, err := pcx.beginDependencyOperation(ctx, "update lockfile")
	if err != nil {
		return err
	}
	defer func() { err = pcx.finishDependencyOperation(state, err) }()

	if err := pcx.UpdateLockfile(ctx, request); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := pcx.saveDependencyLockfile(); err != nil {
		return &OperationError{Operation: "lockfile", Err: err}
	}

	return nil
}
