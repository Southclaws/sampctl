package pkgcontext

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/resource"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
)

// installPackageResources handles resource installation from cached package.
func (pcx *PackageContext) installPackageResources(
	ctx context.Context,
	originalMeta versioning.DependencyMeta,
	installedMeta versioning.DependencyMeta,
) error {
	pkg, err := pcx.resourcePackageDefinition(ctx, installedMeta)
	resourceMeta := pcx.resourceDependencyMeta(originalMeta, installedMeta)
	applyDependencyMetaToPackage(&pkg, resourceMeta)

	for _, resource := range pkg.Resources {
		if resource.Platform != pcx.Platform || len(resource.Includes) == 0 {
			continue
		}

		includePath, err := pcx.extractResourceDependencies(ctx, pkg, resource)
		if err != nil {
			return err
		}
		pcx.AllIncludePaths = append(pcx.AllIncludePaths, includePath)
	}

	return err
}

func (pcx *PackageContext) resourceDependencyMeta(
	originalMeta versioning.DependencyMeta,
	installedMeta versioning.DependencyMeta,
) versioning.DependencyMeta {
	resourceMeta := installedMeta
	locked, hasLocked := pcx.currentLockedDependency(originalMeta)

	switch {
	case originalMeta.Commit != "":
		resourceMeta.Commit = originalMeta.Commit
		resourceMeta.Tag = ""
		resourceMeta.Branch = ""
	case originalMeta.Branch != "":
		resourceMeta.Branch = originalMeta.Branch
		resourceMeta.Tag = ""
		resourceMeta.Commit = ""
	case originalMeta.Tag == "latest":
		resourceMeta.Commit = ""
		resourceMeta.Branch = ""
		if hasLocked && locked.Resolved != "" && locked.Resolved != "HEAD" {
			resourceMeta.Tag = locked.Resolved
		} else {
			resourceMeta.Tag = originalMeta.Tag
		}
	case originalMeta.Tag != "":
		resourceMeta.Tag = originalMeta.Tag
		resourceMeta.Branch = ""
		resourceMeta.Commit = ""
	case hasLocked && locked.Resolved != "" && locked.Resolved != "HEAD":
		resourceMeta.Tag = locked.Resolved
		resourceMeta.Branch = ""
		resourceMeta.Commit = ""
	}

	return resourceMeta
}

func (pcx *PackageContext) currentLockedDependency(meta versioning.DependencyMeta) (lockfile.LockedDependency, bool) {
	lf := pcx.PackageLockfileState.GetLockfile()
	if lf == nil {
		return lockfile.LockedDependency{}, false
	}

	return lf.GetDependency(lockfile.DependencyKey(meta))
}

func (pcx *PackageContext) resourcePackageDefinition(ctx context.Context, meta versioning.DependencyMeta) (pawnpackage.Package, error) {
	// Resource installation needs a package definition (`pawn.json`/`pawn.yaml`). Prefer the cached copy,
	// then the checked-out dependency, and finally the remote definition to avoid dropping include paths.
	pkg, err := pawnpackage.GetCachedPackage(meta, pcx.CacheDir)
	if err != nil {
		print.Verb(meta, "failed to read cached package definition:", err)
	}
	if err == nil && pkg.Format != "" {
		return pkg, nil
	}

	depDir := filepath.Join(pcx.Package.Vendor, meta.Repo)
	pkgLocal, errLocal := pawnpackage.PackageFromDir(depDir)
	if errLocal == nil && pkgLocal.Format != "" {
		print.Verb(meta, "using local dependency package definition for resources")
		return pkgLocal, nil
	}

	if pcx.RemotePackages == nil {
		return pkg, err
	}

	pkgRemote, errRemote := pcx.RemotePackages.Fetch(ctx, meta)
	if errRemote == nil {
		print.Verb(meta, "using remote package definition for resources")
		return pkgRemote, nil
	}

	return pkg, err
}

func applyDependencyMetaToPackage(pkg *pawnpackage.Package, meta versioning.DependencyMeta) {
	if pkg == nil {
		return
	}

	pkg.SetDependencyMeta(meta)
}

func (pcx PackageContext) extractResourceDependencies(
	ctx context.Context,
	pkg pawnpackage.Package,
	res resource.Resource,
) (string, error) {
	dir := filepath.Join(pcx.Package.Vendor, res.Path(pkg.Repo))
	print.Verb(pkg, "installing resource-based dependency", res.Name, "to", dir)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.Wrap(err, "failed to create target directory")
	}

	_, err := runtime.EnsureVersionedPlugin(runtime.EnsureVersionedPluginRequest{
		Context:        ctx,
		GitHub:         pcx.GitHub,
		Meta:           pkg.Dependency(),
		Dir:            dir,
		Platform:       pcx.Platform,
		Version:        res.Version,
		CacheDir:       pcx.CacheDir,
		PluginDestDir:  "",
		Plugins:        false,
		Includes:       true,
		NoCache:        false,
		IgnorePatterns: pcx.Package.ExtractIgnorePatterns,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to ensure asset")
	}

	return dir, nil
}
