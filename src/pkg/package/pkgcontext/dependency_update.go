package pkgcontext

import (
	"context"
	stderrors "errors"

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

	if dependencyUpdateIdentity(meta) != dependencyUpdateIdentity(request.TargetMeta) {
		return false
	}

	if !hasExplicitDependencyConstraint(request.TargetMeta) {
		return true
	}

	return sameDependencyConstraint(meta, request.TargetMeta)
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

func hasExplicitDependencyConstraint(meta versioning.DependencyMeta) bool {
	return meta.Tag != "" || meta.Branch != "" || meta.Commit != ""
}

func sameDependencyConstraint(left versioning.DependencyMeta, right versioning.DependencyMeta) bool {
	return left.Tag == right.Tag && left.Branch == right.Branch && left.Commit == right.Commit
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
) (updated bool, err error) {
	if !request.Enabled || !pcx.Package.Parent {
		return false, nil
	}

	if pcx.Package.LocalPath == "" {
		return false, errors.New("package has no local path")
	}

	standalone := pcx.dependencyInstalls == nil
	if standalone {
		state, beginErr := pcx.beginDependencyOperation(ctx, "update references")
		if beginErr != nil {
			return false, beginErr
		}
		defer func() {
			err = pcx.finishDependencyOperation(state, err)
			if err != nil && !dependencyOperationCommitted(err) {
				updated = false
			}
		}()
	}

	definitionPath, definitionPerm, originalDefinition, err := pcx.readDefinitionSnapshot()
	if err != nil {
		return false, errors.Wrap(err, "failed to read package definition")
	}

	originalDeps := append([]versioning.DependencyString(nil), pcx.Package.Dependencies...)
	originalDev := append([]versioning.DependencyString(nil), pcx.Package.Development...)
	originalResolved := pcx.PackageResolvedState

	changedDeps, err := pcx.updateDependencyList(ctx, pcx.Package.Dependencies, request)
	if err != nil {
		return false, err
	}

	changedDev, err := pcx.updateDependencyList(ctx, pcx.Package.Development, request)
	if err != nil {
		return false, err
	}

	if request.HasTarget() && !changedDeps.matched && !changedDev.matched {
		return false, &DependencyError{Dependency: request.Target, Operation: "select", Err: errors.New("dependency was not found in package definition")}
	}

	if !changedDeps.changed && !changedDev.changed {
		return false, nil
	}

	pcx.Package.Dependencies = changedDeps.updated
	pcx.Package.Development = changedDev.updated

	if err := pcx.writeDependencyDefinition(); err != nil {
		pcx.Package.Dependencies = originalDeps
		pcx.Package.Development = originalDev
		return false, &OperationError{Operation: "manifest", Err: err}
	}

	refreshGraph := pcx.refreshDependencyGraphContext
	if standalone {
		refreshGraph = pcx.refreshDependencyReferenceGraphContext
	}
	if err := refreshGraph(ctx, request); err != nil {
		pcx.Package.Dependencies = originalDeps
		pcx.Package.Development = originalDev
		pcx.PackageResolvedState = originalResolved
		restoreErr := fs.WriteFileAtomic(definitionPath, originalDefinition, fs.PermDirPrivate, definitionPerm)
		if restoreErr != nil {
			return false, stderrors.Join(err, &OperationError{Operation: "rollback", Err: restoreErr})
		}
		return false, errors.Wrap(err, "failed to refresh dependencies after update")
	}

	return true, nil
}

func (pcx *PackageContext) updateDependencyList(
	ctx context.Context,
	deps []versioning.DependencyString,
	request DependencyUpdateRequest,
) (dependencyUpdateResult, error) {
	result := dependencyUpdateResult{updated: make([]versioning.DependencyString, 0, len(deps))}
	var failures []error

	for _, depStr := range deps {
		if err := ctx.Err(); err != nil {
			return dependencyUpdateResult{}, stderrors.Join(append(failures, err)...)
		}

		meta, err := depStr.Explode()
		if err != nil {
			failures = append(failures, &DependencyError{Dependency: string(depStr), Operation: "parse", Err: err})
			continue
		}

		if !request.Matches(meta) {
			result.updated = append(result.updated, depStr)
			continue
		}
		result.matched = true

		updatedMeta, changed, updateErr := pcx.updatedDependencyMeta(ctx, meta, request)
		if updateErr != nil {
			failures = append(failures, &DependencyError{Dependency: string(depStr), Operation: "update", Err: updateErr})
			if stderrors.Is(updateErr, context.Canceled) || stderrors.Is(updateErr, context.DeadlineExceeded) {
				return dependencyUpdateResult{}, stderrors.Join(failures...)
			}
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

	return result, stderrors.Join(failures...)
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
				print.Warn(meta, "has no tags or releases. Keeping dependency unpinned")
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
				print.Warn(meta, "has no tags or releases. Keeping the current pin")
				return meta, false, nil
			}

			return versioning.DependencyMeta{}, false, err
		}
		if tag == "" {
			return versioning.DependencyMeta{}, false, errors.New(":latest did not resolve to a tag")
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
