package pkgcontext

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-git/go-git/v5"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
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

func (pcx *PackageContext) ensureManagedPackage(request ensurePackageRequest) (err error) {
	if err := request.Context.Err(); err != nil {
		return err
	}
	if request.Meta.IsURLScheme() {
		if request.Meta.IsLocalScheme() {
			if err := validateLocalDependencyPath(pcx.Package.LocalPath, request.Meta); err != nil {
				return err
			}
		}
		return pcx.ensureURLSchemeDependency(request.Context, request.Meta)
	}
	if err := validateManagedDependencyName(request.Meta.Repo); err != nil {
		return err
	}

	effectiveMeta := pcx.PackageLockfileState.LockedVersion(request.Meta, request.ForceUpdate)
	effectiveMeta, err = pcx.resolveDynamicDependencyReference(request.Context, effectiveMeta, request.Meta, request.ForceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to resolve dependency reference")
	}
	if err := validateManagedDependencyName(effectiveMeta.Repo); err != nil {
		return err
	}
	state := pcx.dependencyInstalls
	if state == nil {
		state = newDependencyInstallState()
		restoreRecord := pcx.dependencyRecordSnapshot(request.Meta)
		includeCount := len(pcx.AllIncludePaths)
		defer func() {
			if err != nil {
				if rollbackErr := state.rollback(); rollbackErr != nil {
					err = stderrors.Join(err, &OperationError{Operation: "rollback", Err: rollbackErr})
				}
				restoreRecord()
				pcx.AllIncludePaths = pcx.AllIncludePaths[:includeCount]
				return
			}
			if cleanupErr := state.commit(); cleanupErr != nil {
				err = &OperationError{Operation: "cleanup", Err: cleanupErr}
			}
		}()
	}
	return pcx.ensureStagedPackage(request, effectiveMeta, state)
}

func (pcx *PackageContext) dependencyRecordSnapshot(meta versioning.DependencyMeta) func() {
	lf := pcx.GetLockfile()
	if lf == nil {
		return func() {}
	}
	key := lockfile.DependencyKey(meta)
	previous, exists := lf.GetDependency(key)
	previous.RequiredBy = slices.Clone(previous.RequiredBy)
	return func() {
		if exists {
			lf.AddDependency(key, previous)
			return
		}
		lf.RemoveDependency(key)
	}
}

func (pcx *PackageContext) ensureStagedPackage(request ensurePackageRequest, effectiveMeta versioning.DependencyMeta, state *dependencyInstallState) error {
	stage, err := state.stage(pcx.Package.Vendor)
	if err != nil {
		return err
	}
	staged := *pcx
	staged.Package.Vendor = stage
	staged.AllIncludePaths = nil
	dependencyPath := filepath.Join(stage, effectiveMeta.Repo)
	finalPath := filepath.Join(pcx.Package.Vendor, effectiveMeta.Repo)
	if err := stagePreviousDependency(request.Context, finalPath, dependencyPath); err != nil {
		return err
	}
	if err := staged.prepareDependency(request, effectiveMeta, dependencyPath); err != nil {
		return err
	}
	if err := request.Context.Err(); err != nil {
		return err
	}
	if !unchangedDependency(finalPath, dependencyPath, request.ForceUpdate) {
		if err := state.promote(dependencyPath, finalPath); err != nil {
			return err
		}
	}
	return pcx.promoteStagedResources(request.Context, stage, staged.AllIncludePaths, state)
}

func (pcx *PackageContext) promoteStagedResources(ctx context.Context, stage string, includes []string, state *dependencyInstallState) error {
	seen := make(map[string]bool)
	for _, includePath := range includes {
		if seen[includePath] {
			continue
		}
		seen[includePath] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(stage, includePath)
		if err != nil || !filepath.IsLocal(relative) {
			return fmt.Errorf("resource include directory escapes dependency staging: %s", includePath)
		}
		target := filepath.Join(pcx.Package.Vendor, relative)
		if err := state.promote(includePath, target); err != nil {
			return err
		}
		pcx.AllIncludePaths = append(pcx.AllIncludePaths, target)
	}
	return nil
}

func (pcx *PackageContext) prepareDependency(request ensurePackageRequest, effectiveMeta versioning.DependencyMeta, dependencyPath string) error {
	if err := pcx.removeInvalidDependencyRepo(effectiveMeta, dependencyPath); err != nil {
		return err
	}

	repo, err := pcx.ensureDependencyRepositoryContext(request.Context, effectiveMeta, dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependency repository")
	}

	if err := pcx.updateRepoStateWithRecoveryContext(request.Context, repo, effectiveMeta, dependencyPath, request.ForceUpdate); err != nil {
		return errors.Wrap(err, "failed to update repository state")
	}

	refreshIntegrity := request.ForceUpdate
	if lf := pcx.GetLockfile(); lf != nil && lf.IsOutdated(request.Meta) {
		refreshIntegrity = true
	}
	if err := pcx.verifyDependencyIntegrityWithRecoveryContext(request.Context, effectiveMeta, dependencyPath, refreshIntegrity); err != nil {
		return errors.Wrap(err, "failed to verify dependency integrity")
	}

	repo, err = pcx.PackageServices.repositoryStore().Open(dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to reopen dependency repository after integrity verification")
	}

	if err := pcx.installPackageResources(request.Context, request.Meta, effectiveMeta); err != nil {
		return errors.Wrap(err, "failed to install package resources")
	}
	return pcx.recordDependencyResolution(request.Meta, request.ParentRepo, repo)
}

func stagePreviousDependency(ctx context.Context, source, destination string) error {
	info, err := os.Lstat(source)
	if stderrors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing dependency: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("existing dependency %s must be a directory, not a symbolic link or file", source)
	}
	gitDirectory := filepath.Join(source, ".git")
	if _, err := os.Lstat(gitDirectory); stderrors.Is(err, os.ErrNotExist) {
		return validateRepositoryDestination(source)
	}
	if err := filepath.WalkDir(gitDirectory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("dependency Git metadata contains a symbolic link: %s", path)
		}
		return ctx.Err()
	}); err != nil {
		return fmt.Errorf("cannot safely stage existing dependency %s: %w", source, err)
	}
	if err := copyDependencyForStaging(ctx, source, destination); err != nil {
		return fmt.Errorf("stage existing dependency %s: %w", source, err)
	}
	return nil
}

func unchangedDependency(existing, prepared string, force bool) bool {
	if force {
		return false
	}
	oldRepo, err := git.PlainOpen(existing)
	if err != nil {
		return false
	}
	newRepo, err := git.PlainOpen(prepared)
	if err != nil {
		return false
	}
	oldHead, err := oldRepo.Head()
	if err != nil {
		return false
	}
	newHead, err := newRepo.Head()
	if err != nil || oldHead.Hash() != newHead.Hash() {
		return false
	}
	worktree, err := oldRepo.Worktree()
	if err != nil {
		return false
	}
	status, err := worktree.Status()
	return err == nil && status.IsClean()
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
	print.Verb(meta, "removing invalid repository for fresh clone")
	if err := os.RemoveAll(dependencyPath); err != nil {
		return errors.Wrap(err, "failed to remove invalid dependency repo")
	}

	return nil
}

func (pcx *PackageContext) recordDependencyResolution(meta versioning.DependencyMeta, parentRepo string, repo *git.Repository) error {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return nil
	}

	isTransitive := parentRepo != "" && parentRepo != pcx.Package.Repo
	resolution, err := resolveDependencyLock(meta, repo)
	if err != nil {
		return errors.Wrap(err, "failed to resolve dependency lock data")
	}

	if err := pcx.PackageLockfileState.RecordDependencyResolution(meta, resolution, isTransitive, parentRepo); err != nil {
		return errors.Wrap(err, "failed to record dependency resolution to lockfile")
	}
	return nil
}
