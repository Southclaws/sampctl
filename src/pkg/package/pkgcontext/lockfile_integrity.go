package pkgcontext

import (
	"context"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func (pcx *PackageContext) verifyDependencyIntegrityWithRecoveryContext(ctx context.Context, meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if forceUpdate {
		return nil
	}

	if err := pcx.verifyLockedDependencyIntegrityContext(ctx, meta, dependencyPath); err == nil {
		return nil
	} else {
		print.Verb(meta, "dependency integrity check failed, re-cloning")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	repo, err := pcx.recloneDependencyContext(ctx, meta, dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to re-clone dependency after integrity mismatch")
	}

	if err := pcx.updateRepoStateWithRecoveryContext(ctx, repo, meta, dependencyPath, forceUpdate); err != nil {
		return errors.Wrap(err, "failed to recover dependency after integrity mismatch")
	}

	if err := pcx.verifyLockedDependencyIntegrityContext(ctx, meta, dependencyPath); err != nil {
		return errors.Wrap(err, "integrity mismatch after recovery")
	}

	return nil
}

func (pcx *PackageContext) verifyLockedDependencyIntegrityContext(ctx context.Context, meta versioning.DependencyMeta, dependencyPath string) error {
	if !pcx.PackageLockfileState.HasLockfileResolver() {
		return nil
	}

	lf := pcx.PackageLockfileState.GetLockfile()
	if lf == nil {
		return nil
	}

	locked, ok := lf.GetDependency(lockfile.DependencyKey(meta))
	if !ok || locked.Integrity == "" {
		return nil
	}

	ok, err := lockfile.VerifyIntegrityContext(ctx, dependencyPath, locked.Integrity)
	if err != nil {
		return errors.Wrap(err, "failed to verify locked dependency integrity")
	}
	if !ok {
		return errors.New("locked dependency integrity mismatch")
	}

	return nil
}
