package pkgcontext

import (
	"context"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// DependencyUpdateRequest describes how an ensure/update flow should refresh direct dependencies.
type DependencyUpdateRequest struct {
	Enabled    bool
	Force      bool
	Target     string
	TargetMeta versioning.DependencyMeta
}

func (request DependencyUpdateRequest) HasTarget() bool {
	return request.Target != ""
}

func (request DependencyUpdateRequest) Matches(meta versioning.DependencyMeta) bool {
	if !request.HasTarget() {
		return true
	}

	return dependencyUpdateIdentity(meta) == dependencyUpdateIdentity(request.TargetMeta)
}

func (request DependencyUpdateRequest) ShouldForceDependency(meta versioning.DependencyMeta, direct bool) bool {
	if !request.Enabled {
		return false
	}
	if request.Force && !request.HasTarget() {
		return true
	}
	if !direct || !request.Matches(meta) {
		return false
	}

	if isDynamicDependencyConstraint(meta) {
		return true
	}

	return request.Force && isPinnedTagDependency(meta)
}

func dependencyUpdateIdentity(meta versioning.DependencyMeta) string {
	if meta.Scheme != "" {
		if meta.Local != "" {
			return meta.Scheme + "://local/" + meta.Local
		}
		return meta.Scheme + "://" + normalizeDependencySite(meta.Site) + "/" + meta.User + "/" + meta.Repo + "/" + meta.Path
	}

	return normalizeDependencySite(meta.Site) + "/" + meta.User + "/" + meta.Repo + "/" + meta.Path
}

func normalizeDependencySite(site string) string {
	if site == "" {
		return "github.com"
	}

	return site
}

func isDynamicDependencyConstraint(meta versioning.DependencyMeta) bool {
	return meta.Tag == "latest" || (meta.Tag == "" && meta.Branch == "" && meta.Commit == "")
}

func isPinnedTagDependency(meta versioning.DependencyMeta) bool {
	return meta.Tag != "" && meta.Tag != "latest" && meta.Branch == "" && meta.Commit == ""
}

type dependencyUpdateResult struct {
	updated []versioning.DependencyString
	changed bool
	matched bool
}

// UpdateDependencyReferences rewrites direct dependency constraints for an explicit update request.
func (pcx *PackageContext) UpdateDependencyReferences(
	ctx context.Context,
	request DependencyUpdateRequest,
) (bool, error) {
	if !request.Enabled || !pcx.Package.Parent {
		return false, nil
	}
	if pcx.Package.LocalPath == "" {
		return false, errors.New("package has no local path")
	}

	definitionPath, definitionPerm, originalDefinition, err := pcx.readDefinitionSnapshot()
	if err != nil {
		return false, errors.Wrap(err, "failed to read package definition")
	}

	originalDeps := append([]versioning.DependencyString(nil), pcx.Package.Dependencies...)
	originalDev := append([]versioning.DependencyString(nil), pcx.Package.Development...)

	changedDeps, err := pcx.updateDependencyList(ctx, pcx.Package.Dependencies, request)
	if err != nil {
		return false, err
	}
	changedDev, err := pcx.updateDependencyList(ctx, pcx.Package.Development, request)
	if err != nil {
		return false, err
	}

	if request.HasTarget() && !changedDeps.matched && !changedDev.matched {
		return false, errors.Errorf("dependency %s was not found in package definition", request.Target)
	}

	if !changedDeps.changed && !changedDev.changed {
		return false, nil
	}

	pcx.Package.Dependencies = changedDeps.updated
	pcx.Package.Development = changedDev.updated

	if err := pcx.Package.WriteDefinition(); err != nil {
		return false, errors.Wrap(err, "failed to write updated package definition")
	}

	if err := pcx.EnsureDependenciesCached(); err != nil {
		pcx.Package.Dependencies = originalDeps
		pcx.Package.Development = originalDev
		restoreErr := fs.WriteFileAtomic(definitionPath, originalDefinition, fs.PermDirPrivate, definitionPerm)
		if restoreErr != nil {
			return false, errors.Wrapf(err, "failed to refresh dependency tree after update, rollback failed: %v", restoreErr)
		}
		return false, errors.Wrap(err, "failed to refresh dependency tree after update, rolling back changes")
	}

	return true, nil
}

func (pcx *PackageContext) updateDependencyList(
	ctx context.Context,
	deps []versioning.DependencyString,
	request DependencyUpdateRequest,
) (dependencyUpdateResult, error) {
	result := dependencyUpdateResult{updated: make([]versioning.DependencyString, 0, len(deps))}

	for _, depStr := range deps {
		meta, err := depStr.Explode()
		if err != nil {
			print.Warn("invalid dependency string, skipping update:", depStr, "(", err, ")")
			result.updated = append(result.updated, depStr)
			continue
		}

		if !request.Matches(meta) {
			result.updated = append(result.updated, depStr)
			continue
		}
		result.matched = true

		updatedMeta, changed, updateErr := pcx.updatedDependencyMeta(ctx, meta, request)
		if updateErr != nil {
			if request.HasTarget() {
				return dependencyUpdateResult{}, updateErr
			}

			print.Warn(meta, "failed to update dependency reference:", updateErr)
			result.updated = append(result.updated, depStr)
			continue
		}

		if !changed {
			result.updated = append(result.updated, depStr)
			continue
		}

		newDep := versioning.DependencyString(formatPinnedDependency(updatedMeta))
		result.updated = append(result.updated, newDep)
		result.changed = true
		print.Verb("updated dependency", depStr, "->", newDep)
	}

	return result, nil
}

func (pcx *PackageContext) updatedDependencyMeta(
	ctx context.Context,
	meta versioning.DependencyMeta,
	request DependencyUpdateRequest,
) (versioning.DependencyMeta, bool, error) {
	if meta.IsLocalScheme() {
		return meta, false, nil
	}
	if meta.User == "" || meta.Repo == "" {
		return meta, false, nil
	}

	switch {
	case meta.Tag == "" && meta.Branch == "" && meta.Commit == "":
		if _, err := pcx.resolveLatestTag(ctx, meta, true); err != nil {
			if isMissingLatestReleaseError(err) {
				print.Warn(meta, "does not publish tags or releases, leaving dependency unpinned")
				return meta, false, nil
			}

			return versioning.DependencyMeta{}, false, err
		}

		updatedMeta := meta
		updatedMeta.Tag = "latest"
		return updatedMeta, true, nil
	case meta.Tag == "latest":
		return meta, false, nil
	case request.Force && isPinnedTagDependency(meta):
		tag, err := pcx.resolveLatestTag(ctx, meta, true)
		if err != nil {
			if isMissingLatestReleaseError(err) {
				print.Warn(meta, "does not publish tags or releases, leaving pinned dependency unchanged")
				return meta, false, nil
			}

			return versioning.DependencyMeta{}, false, err
		}
		if tag == "" {
			return versioning.DependencyMeta{}, false, errors.New("latest did not resolve to a concrete tag")
		}

		updatedMeta := meta
		updatedMeta.Tag = tag
		return updatedMeta, updatedMeta.Tag != meta.Tag, nil
	default:
		return meta, false, nil
	}
}

func (pcx *PackageContext) directDependencySet() map[string]struct{} {
	direct := make(map[string]struct{}, len(pcx.Package.Dependencies)+len(pcx.Package.Development))

	for _, depStr := range append(append([]versioning.DependencyString(nil), pcx.Package.Dependencies...), pcx.Package.Development...) {
		meta, err := depStr.Explode()
		if err != nil {
			continue
		}

		direct[dependencyUpdateIdentity(meta)] = struct{}{}
	}

	return direct
}
