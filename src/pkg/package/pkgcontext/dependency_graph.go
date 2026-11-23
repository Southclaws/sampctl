package pkgcontext

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func (pcx *PackageContext) EnsureDependenciesCached() error {
	return pcx.EnsureDependenciesCachedContext(context.Background())
}

// EnsureDependenciesCachedContext visits dependencies in the shared cache and respects cancellation.
func (pcx *PackageContext) EnsureDependenciesCachedContext(ctx context.Context) error {
	return pcx.refreshDependencyGraphContext(ctx, DependencyUpdateRequest{})
}

func (pcx *PackageContext) refreshDependencyGraphContext(ctx context.Context, request DependencyUpdateRequest) error {
	return pcx.refreshDependencyGraphWithResolution(ctx, request, dependencyGraphOptions{})
}

// Reference-only updates can name tags that are not cached yet.
func (pcx *PackageContext) refreshDependencyReferenceGraphContext(ctx context.Context, request DependencyUpdateRequest) error {
	return pcx.refreshDependencyGraphWithResolution(ctx, request, dependencyGraphOptions{referenceOnly: true})
}

type dependencyGraphOptions struct {
	referenceOnly bool
	policy        *versioning.ProjectOverrides
	activePins    map[versioning.DependencyMeta]bool
}

func (pcx *PackageContext) refreshDependencyGraphWithResolution(ctx context.Context, request DependencyUpdateRequest, options dependencyGraphOptions) error {
	if !pcx.Package.Parent {
		return errors.New("package is not a parent package")
	}

	if pcx.Package.LocalPath == "" {
		return errors.New("package has no known local path")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	var policy versioning.ProjectOverrides
	if options.policy == nil {
		var err error
		policy, err = versioning.NewProjectOverrides(pcx.Package.Overrides, pcx.Package.Format)
		if err != nil {
			return err
		}
	} else {
		policy = *options.policy
	}
	pcx.staleOverrideLocks = nil
	for {
		retry, err := pcx.walkDependencyGraph(ctx, request, options, policy)
		if !retry || ctx.Err() != nil {
			return err
		}
	}
}

func (pcx *PackageContext) walkDependencyGraph(ctx context.Context, request DependencyUpdateRequest, options dependencyGraphOptions, policy versioning.ProjectOverrides) (bool, error) {
	selection := request.selectDependencies(policy, pcx.Package.GetAllDependencies())
	retry := false
	pcx.AllDependencies = nil
	pcx.AllPlugins = nil
	pcx.AllIncludePaths = nil
	visited := map[versioning.DependencyMeta]bool{managedDependencyIdentity(pcx.Package.Dependency()): true}
	directDependencies := make(map[versioning.DependencyMeta]bool)
	directConstraints := pcx.directDependencyConstraints(policy)
	for _, declaration := range pcx.Package.GetAllDependencies() {
		meta, err := policy.Resolve(declaration)
		if err == nil {
			directDependencies[managedDependencyIdentity(meta)] = true
		}
	}

	destinations := make(map[string]versioning.DependencyMeta)
	var recurse func(versioning.DependencyMeta, bool, int, bool) error
	recurse = func(meta versioning.DependencyMeta, direct bool, depth int, inferPlugins bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		prefix := strings.Repeat("|-", depth+1)
		currentPackage := pcx.Package
		if depth != 0 {
			forceUpdate := selection.ShouldForceDependency(meta, direct || directDependencies[managedDependencyIdentity(meta)])
			var err error
			currentPackage, err = pcx.loadDependencyPackage(ctx, meta, forceUpdate, !options.referenceOnly)
			if err != nil {
				return err
			}

			pcx.AllDependencies = append(pcx.AllDependencies, meta)
			pcx.recordCachedPackageResources(currentPackage, meta, prefix, inferPlugins)
		}

		dependencies := currentPackage.Dependencies
		if depth == 0 {
			dependencies = currentPackage.GetAllDependencies()
		}
		var failures []error
		for _, dependency := range dependencies {
			if err := ctx.Err(); err != nil {
				return stderrors.Join(append(failures, err)...)
			}

			subMeta, err := policy.Resolve(dependency)
			if err != nil {
				failures = append(failures, &DependencyError{Dependency: string(dependency), Operation: "parse", Err: errors.Wrapf(err, "invalid dependency in %s", currentPackage)})
				continue
			}
			if isPinnedTagDependency(subMeta) && policy.Matches(dependency) {
				identity := managedDependencyIdentity(subMeta)
				if options.activePins != nil {
					options.activePins[identity] = true
				}
				if !pcx.staleOverrideLocks[identity] {
					if _, stale := pcx.GetLockfile().ConflictingOverrideTag(normalizeLockfileDependency(subMeta)); stale {
						if pcx.staleOverrideLocks == nil {
							pcx.staleOverrideLocks = make(map[versioning.DependencyMeta]bool)
						}
						pcx.staleOverrideLocks[identity] = true
						retry = retry || visited[identity]
					}
				}
			}
			subMeta = inheritDirectDependencyConstraint(subMeta, directConstraints)
			if options.activePins == nil && !subMeta.IsLocalScheme() {
				if err := registerDependencyDestination(destinations, subMeta); err != nil {
					failures = append(failures, err)
					continue
				}
			}

			if subMeta.IsLocalScheme() && policy.Matches(dependency) {
				if err := validateLocalDependencyPath(pcx.Package.LocalPath, subMeta); err != nil {
					failures = append(failures, &DependencyError{Dependency: string(dependency), Operation: "access override local path", Err: errors.Wrapf(err, "replacement %s from pawn.%s overrides", subMeta, pcx.Package.Format)})
					continue
				}
			}

			inferPlugins := !subMeta.IsURLScheme()
			if subMeta.IsURLScheme() {
				err = pcx.handleURLSchemeCaching(subMeta, prefix)
				if err != nil {
					failures = append(failures, &DependencyError{Dependency: subMeta.String(), Operation: "cache", Err: err})
					continue
				}
				if subMeta.IsLocalScheme() {
					continue
				}
				subMeta = normalizeLockfileDependency(subMeta)
			}
			if identity := managedDependencyIdentity(subMeta); !visited[identity] {
				visited[identity] = true
				err = recurse(subMeta, depth == 0, depth+1, inferPlugins)
			}
			if err != nil {
				var dependencyErr *DependencyError
				if !stderrors.As(err, &dependencyErr) {
					err = &DependencyError{Dependency: subMeta.String(), Operation: "cache", Err: err}
				}
				failures = append(failures, err)
				if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
					return stderrors.Join(failures...)
				}
			}
		}

		return stderrors.Join(failures...)
	}

	err := recurse(pcx.Package.Dependency(), false, 0, true)
	return retry, err
}

// activeProjectPins finds the fixed tags actually selected by project rules.
func (pcx PackageContext) activeProjectPins(ctx context.Context, request DependencyUpdateRequest, policy versioning.ProjectOverrides) (map[versioning.DependencyMeta]bool, error) {
	if !request.Enabled || !request.Force || !policy.HasPinnedTags() {
		return nil, nil
	}
	pins := make(map[versioning.DependencyMeta]bool)
	err := pcx.refreshDependencyGraphWithResolution(ctx, DependencyUpdateRequest{}, dependencyGraphOptions{policy: &policy, activePins: pins})
	return pins, err
}

func (pcx *PackageContext) loadDependencyPackage(ctx context.Context, meta versioning.DependencyMeta, forceUpdate, resolveLatest bool) (pawnpackage.Package, error) {
	effectiveMeta := pcx.LockedVersion(meta, forceUpdate || pcx.staleOverrideLocks[managedDependencyIdentity(meta)])
	if resolveLatest {
		resolvedMeta, err := pcx.resolveDynamicDependencyReference(ctx, effectiveMeta, meta, forceUpdate)
		if err != nil {
			return pawnpackage.Package{}, &DependencyError{Dependency: meta.String(), Operation: "resolve", Err: err}
		}
		effectiveMeta = resolvedMeta
	}

	refreshCache := forceUpdate
	if refreshCache {
		noOrigin, err := pcx.cachedRepoHasNoOriginContext(ctx, effectiveMeta)
		if err != nil {
			return pawnpackage.Package{}, &DependencyError{Dependency: meta.String(), Operation: "cache", Err: err}
		}
		refreshCache = !noOrigin
	}
	if _, err := pcx.EnsureDependencyCachedContext(ctx, effectiveMeta, refreshCache); err != nil {
		return pawnpackage.Package{}, &DependencyError{Dependency: meta.String(), Operation: "cache", Err: err}
	}

	pkg, err := pcx.packageFromCachedRevisionContext(ctx, effectiveMeta, effectiveMeta.CachePath(pcx.CacheDir))
	if err != nil {
		return pawnpackage.Package{}, &DependencyError{Dependency: meta.String(), Operation: "read package metadata", Err: err}
	}

	return pkg, nil
}

func (pcx *PackageContext) recordCachedPackageResources(pkg pawnpackage.Package, meta versioning.DependencyMeta, prefix string, inferPlugins bool) {
	isPlugin := false
	for _, resource := range pkg.Resources {
		if resource.Platform == pcx.Platform && len(resource.Includes) > 0 {
			targetPath := filepath.Join(pcx.Package.Vendor, resource.Path(pkg.Repo))
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, targetPath)
			print.Verb(prefix, "added target path for resource includes:", targetPath)
		}
		if resource.Archive {
			isPlugin = isPlugin || len(resource.Plugins) > 0
		} else {
			isPlugin = isPlugin || strings.Contains(resource.Name, "dll") || strings.Contains(resource.Name, "so")
		}
	}
	if isPlugin && inferPlugins {
		pcx.AllPlugins = append(pcx.AllPlugins, meta)
	}
}

// handleURLSchemeCaching handles URL scheme dependencies during the caching phase
func (pcx *PackageContext) handleURLSchemeCaching(meta versioning.DependencyMeta, prefix string) error {
	switch meta.Scheme {
	case "plugin":
		if meta.IsLocalScheme() {
			pluginMeta := versioning.DependencyMeta{
				Scheme: "plugin",
				Local:  meta.Local,
				User:   "local",
				Repo:   filepath.Base(meta.Local),
			}
			pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
			print.Verb(prefix, "added local plugin:", meta.Local)
		} else {
			ensureMeta := normalizeLockfileDependency(meta)
			pluginMeta := ensureMeta
			pluginMeta.Scheme = "plugin"
			pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
			print.Verb(prefix, "added remote plugin dependency:", ensureMeta)
		}

	case "component":
		if meta.IsLocalScheme() {
			componentMeta := versioning.DependencyMeta{
				Scheme: "component",
				Local:  meta.Local,
				User:   "local",
				Repo:   filepath.Base(meta.Local),
			}
			pcx.AllPlugins = append(pcx.AllPlugins, componentMeta)
			print.Verb(prefix, "added local component:", meta.Local)
		} else {
			ensureMeta := normalizeLockfileDependency(meta)
			componentMeta := ensureMeta
			componentMeta.Scheme = "component"
			pcx.AllPlugins = append(pcx.AllPlugins, componentMeta)
			print.Verb(prefix, "added remote component dependency:", ensureMeta)
		}

	case "includes":
		if meta.IsLocalScheme() {
			includesPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
			print.Verb(prefix, "added local includes path:", includesPath)
		} else {
			remoteMeta := normalizeLockfileDependency(meta)
			includesPath := filepath.Join(pcx.Package.Vendor, remoteMeta.Repo)
			if remoteMeta.Path != "" {
				includesPath = filepath.Join(includesPath, remoteMeta.Path)
			}
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
			print.Verb(prefix, "added remote includes dependency:", remoteMeta)
		}

	case "filterscript":
		if meta.IsLocalScheme() {
			print.Verb(prefix, "added local filterscript:", meta.Local)
		} else {
			remoteMeta := normalizeLockfileDependency(meta)
			print.Verb(prefix, "added remote filterscript dependency:", remoteMeta)
		}

	default:
		return errors.Errorf("unsupported URL scheme: %s", meta.Scheme)
	}

	return nil
}
