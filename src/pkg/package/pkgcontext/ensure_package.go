package pkgcontext

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type ensurePackageRequest struct {
	Context     context.Context
	Meta        versioning.DependencyMeta
	ForceUpdate bool
	ParentRepo  string
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD.
// If the package is present, it will ensure the directory contains the correct version.
// When lockfile support is enabled, it uses locked versions for reproducibility.
func (pcx *PackageContext) EnsurePackage(meta versioning.DependencyMeta, forceUpdate bool) error {
	return pcx.ensurePackage(context.Background(), meta, forceUpdate)
}

func (pcx *PackageContext) ensurePackage(ctx context.Context, meta versioning.DependencyMeta, forceUpdate bool) error {
	return pcx.ensureManagedPackage(ensurePackageRequest{
		Context:     ctx,
		Meta:        meta,
		ForceUpdate: forceUpdate,
	})
}

// EnsurePackageWithParent ensures a package and records it as a transitive dependency.
func (pcx *PackageContext) EnsurePackageWithParent(meta versioning.DependencyMeta, forceUpdate bool, parentRepo string) error {
	return pcx.ensurePackageWithParent(context.Background(), meta, forceUpdate, parentRepo)
}

func (pcx *PackageContext) ensurePackageWithParent(
	ctx context.Context,
	meta versioning.DependencyMeta,
	forceUpdate bool,
	parentRepo string,
) error {
	return pcx.ensureManagedPackage(ensurePackageRequest{
		Context:     ctx,
		Meta:        meta,
		ForceUpdate: forceUpdate,
		ParentRepo:  parentRepo,
	})
}

func (pcx *PackageContext) ensureManagedPackage(request ensurePackageRequest) error {
	if request.Meta.IsURLScheme() {
		return pcx.ensureURLSchemeDependency(request.Context, request.Meta)
	}

	effectiveMeta := pcx.PackageLockfileState.LockedVersion(request.Meta, request.ForceUpdate)
	effectiveMeta, err := pcx.resolveDynamicDependencyReference(request.Context, effectiveMeta, request.Meta, request.ForceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to resolve dependency reference")
	}
	dependencyPath := filepath.Join(pcx.Package.Vendor, effectiveMeta.Repo)

	if err := pcx.removeInvalidDependencyRepo(effectiveMeta, dependencyPath); err != nil {
		return err
	}

	repo, err := pcx.ensureDependencyRepository(effectiveMeta, dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependency repository")
	}

	if err := pcx.updateRepoStateWithRecovery(repo, effectiveMeta, dependencyPath, request.ForceUpdate); err != nil {
		return errors.Wrap(err, "failed to update repository state")
	}

	pcx.recordDependencyResolution(request.Meta, request.ParentRepo, repo)

	if err := pcx.installPackageResources(request.Context, effectiveMeta); err != nil {
		return errors.Wrap(err, "failed to install package resources")
	}

	return nil
}

func (pcx *PackageContext) removeInvalidDependencyRepo(meta versioning.DependencyMeta, dependencyPath string) error {
	if !fs.Exists(dependencyPath) {
		return nil
	}

	valid, validationErr := pcx.PackageServices.repositoryHealth().Validate(dependencyPath)
	if validationErr == nil && valid {
		return nil
	}

	print.Verb(meta, "existing repository is invalid or corrupted")
	if validationErr != nil {
		print.Verb(meta, "validation error:", validationErr)
	}
	print.Verb(meta, "removing invalid repository for fresh clone")
	if err := os.RemoveAll(dependencyPath); err != nil {
		return errors.Wrap(err, "failed to remove invalid dependency repo")
	}

	return nil
}

func (pcx *PackageContext) recordDependencyResolution(meta versioning.DependencyMeta, parentRepo string, repo *git.Repository) {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return
	}

	isTransitive := parentRepo != "" && parentRepo != pcx.Package.Repo
	resolution, err := resolveDependencyLock(meta, repo)
	if err != nil {
		print.Warn("failed to resolve dependency lock data:", err)
		return
	}

	if err := pcx.PackageLockfileState.RecordDependencyResolution(meta, resolution, isTransitive, parentRepo); err != nil {
		print.Warn("failed to record dependency resolution to lockfile:", err)
	}
}
