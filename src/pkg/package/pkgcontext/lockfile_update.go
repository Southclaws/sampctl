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

	pcx.pruneLockfileDependencies(lockfileDependencyMetas(deps))
	pcx.updateLockfileRelationships(deps)
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

	if forceUpdate && isPinnedTagDependency(meta) {
		tag, err := pcx.resolveLatestTag(ctx, meta, true)
		if err != nil {
			if isMissingLatestReleaseError(err) {
				print.Warn(meta, "has no tags or releases; keeping the lockfile pin")
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

func (pcx *PackageContext) currentLockfileDependencies() ([]lockfileDependencyState, error) {
	return pcx.lockfileDependencies(context.Background(), DependencyUpdateRequest{}, false)
}

func (pcx *PackageContext) lockfileDependencies(ctx context.Context, request DependencyUpdateRequest, refresh bool) ([]lockfileDependencyState, error) {
	states := make(map[string]*lockfileDependencyState)
	visited := make(map[string]bool)
	directDependencies := make(map[string]bool)
	directConstraints := pcx.directDependencyConstraints()
	destinations := make(map[string]versioning.DependencyMeta)
	for _, declaration := range pcx.Package.GetAllDependencies() {
		if meta, err := declaration.Explode(); err == nil {
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
			meta, err := depStr.Explode()
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

			if visited[key] {
				continue
			}
			visited[key] = true
			if normalized.IsLocalScheme() {
				if refresh {
					if err := pcx.RecordLocalDependency(normalized); err != nil {
						failures = append(failures, &DependencyError{Dependency: string(depStr), Operation: "record lockfile", Err: err})
					}
				}
				continue
			}

			var pkg pawnpackage.Package
			if refresh {
				rootDependency := directDependencies[key]
				pkg, err = pcx.refreshLockfileDependency(ctx, normalized, request.ShouldForceDependency(normalized, rootDependency), rootDependency, parent)
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

			nextDeps := pkg.Dependencies
			if pkg.Parent {
				nextDeps = pkg.GetAllDependencies()
			}
			if err := walk(nextDeps, key, false); err != nil {
				failures = append(failures, err)
				if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
					return stderrors.Join(failures...)
				}
			}
		}
		return stderrors.Join(failures...)
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

func (pcx *PackageContext) refreshLockfileDependency(ctx context.Context, meta versioning.DependencyMeta, force, direct bool, parent string) (pawnpackage.Package, error) {
	resolved, err := pcx.resolveLockfileDependencyMeta(ctx, meta, force)
	if err != nil {
		return pawnpackage.Package{}, err
	}
	refreshCache := force
	if refreshCache {
		noOrigin, err := pcx.cachedRepoHasNoOriginContext(ctx, resolved)
		if err != nil {
			return pawnpackage.Package{}, err
		}
		refreshCache = !noOrigin
	}
	if _, err := pcx.EnsureDependencyCachedContext(ctx, resolved, refreshCache); err != nil {
		return pawnpackage.Package{}, err
	}
	path := resolved.CachePath(pcx.CacheDir)
	unlock, err := lockRepositoryPath(ctx, path)
	if err != nil {
		return pawnpackage.Package{}, err
	}
	repo, err := pcx.repositoryStore().Open(path)
	var resolution lockfile.DependencyResolution
	if err == nil {
		resolution, err = resolveCachedDependencyLock(resolved, repo)
	}
	err = stderrors.Join(err, unlock())
	if err != nil {
		return pawnpackage.Package{}, err
	}
	if previous, ok := pcx.PreviousDependency(meta); ok && resolved.Commit != "" && previous.Commit == resolution.Commit && previous.Resolved != "" {
		resolution.Resolved = previous.Resolved
	}
	if err := pcx.RecordDependencyResolution(meta, resolution, !direct, parent); err != nil {
		return pawnpackage.Package{}, err
	}
	resolved.Tag, resolved.Branch, resolved.Commit = "", "", resolution.Commit
	return pcx.packageFromCachedRevisionContext(ctx, resolved, path)
}

func (pcx *PackageContext) lockfileDependencyPackage(meta versioning.DependencyMeta) (pawnpackage.Package, error) {
	return pcx.lockfileDependencyPackageContext(context.Background(), meta)
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
	for _, depStr := range pcx.Package.GetAllDependencies() {
		meta, err := depStr.Explode()
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
