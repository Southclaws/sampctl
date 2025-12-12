package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/eapache/go-resiliency.v1/retrier"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	"github.com/Southclaws/sampctl/src/pkg/runtime/runtime"
	"github.com/Southclaws/sampctl/src/resource"
)

// ErrNotRemotePackage describes a repository that does not contain a package definition file
var ErrNotRemotePackage = errors.New("remote repository does not declare a package")

// EnsureDependencies traverses package dependencies and ensures they are up to date.
func (pcx *PackageContext) EnsureDependencies(ctx context.Context, forceUpdate bool) (err error) {
	return pcx.EnsureDependenciesWithRuntime(ctx, forceUpdate, false)
}

// EnsureDependenciesWithRuntime traverses package dependencies and ensures they are up to date.
func (pcx *PackageContext) EnsureDependenciesWithRuntime(ctx context.Context, forceUpdate bool, setupRuntime bool) (err error) {
	if pcx.Package.LocalPath == "" {
		return errors.New("package does not represent a locally stored package")
	}

	if !util.Exists(pcx.Package.LocalPath) {
		return errors.New("package local path does not exist")
	}

	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")

	for _, dependency := range pcx.AllDependencies {
		dep := dependency
		r := retrier.New(retrier.ConstantBackoff(1, 100*time.Millisecond), nil)
		err := r.Run(func() error {
			print.Verb("attempting to ensure dependency", dep)
			errInner := pcx.EnsurePackage(dep, forceUpdate)
			if errInner != nil {
				print.Warn(errors.Wrapf(errInner, "failed to ensure package %s", dep))
				return errInner
			}
			print.Info(pcx.Package, "successfully ensured dependency files for", dep)
			return nil
		})
		if err != nil {
			print.Warn("failed to ensure package", dep, "after 2 attempts, skipping")
			continue
		}
	}

	if pcx.Package.Local && setupRuntime {
		print.Verb(pcx.Package, "package is local, ensuring binaries too")
		pcx.ActualRuntime.WorkingDir = pcx.Package.LocalPath
		pcx.ActualRuntime.Format = pcx.Package.Format

		pcx.ActualRuntime.PluginDeps, err = pcx.GatherPlugins()
		if err != nil {
			return
		}
		run.ApplyRuntimeDefaults(&pcx.ActualRuntime)
		err = runtime.Ensure(ctx, pcx.GitHub, &pcx.ActualRuntime, false)
		if err != nil {
			return
		}
	}

	return err
}

// GatherPlugins iterates the AllPlugins list and appends them to the runtime dependencies list
func (pcx *PackageContext) GatherPlugins() (pluginDeps []versioning.DependencyMeta, err error) {
	print.Verb(pcx.Package, "gathering", len(pcx.AllPlugins), "plugins from package context")
	for _, pluginMeta := range pcx.AllPlugins {
		print.Verb("read plugin from dependency:", pluginMeta)
		pluginDeps = append(pluginDeps, pluginMeta)
	}
	print.Verb(pcx.Package, "gathered plugins:", pluginDeps)
	return
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func (pcx *PackageContext) EnsurePackage(meta versioning.DependencyMeta, forceUpdate bool) error {
	// Handle URL-like schemes differently
	if meta.IsURLScheme() {
		return pcx.ensureURLSchemeDependency(meta)
	}

	dependencyPath := filepath.Join(pcx.Package.Vendor, meta.Repo)

	if util.Exists(dependencyPath) {
		valid, validationErr := ValidateRepository(dependencyPath)
		if validationErr != nil || !valid {
			print.Verb(meta, "existing repository is invalid or corrupted")
			if validationErr != nil {
				print.Verb(meta, "validation error:", validationErr)
			}
			print.Verb(meta, "removing invalid repository for fresh clone")
			err := os.RemoveAll(dependencyPath)
			if err != nil {
				return errors.Wrap(err, "failed to remove invalid dependency repo")
			}
		}
	}

	repo, err := pcx.ensureDependencyRepository(meta, dependencyPath, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependency repository")
	}

	err = pcx.updateRepoStateWithRecovery(repo, meta, dependencyPath, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to update repository state")
	}

	err = pcx.installPackageResources(meta)
	if err != nil {
		return errors.Wrap(err, "failed to install package resources")
	}

	return nil
}

// installPackageResources handles resource installation from cached package
func (pcx *PackageContext) installPackageResources(meta versioning.DependencyMeta) error {
	// cloned copy of the package that resides in `dependencies/` because that repository may be
	// checked out to a commit that existed before a `pawn.json` file was added that describes where
	// resources can be downloaded from. Therefore, we instead instantiate a new pawnpackage.Package from
	// the cached version of the package because the cached copy is always at the latest version, or
	// at least guaranteed to be either later or equal to the local dependency version.
	pkg, err := pawnpackage.GetCachedPackage(meta, pcx.CacheDir)
	if err != nil {
		return err
	}

	// But the cached copy will have the latest tag assigned to it, so before ensuring it, apply the
	// tag of the actual package we installed.
	pkg.Tag = meta.Tag

	var includePath string
	for _, resource := range pkg.Resources {
		if resource.Platform != pcx.Platform {
			continue
		}

		if len(resource.Includes) > 0 {
			includePath, err = pcx.extractResourceDependencies(context.Background(), pkg, resource)
			if err != nil {
				return err
			}
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includePath)
		}
	}

	return err
}

// ensureURLSchemeDependency handles dependencies with URL-like schemes (plugin://, includes://, filterscript://)
func (pcx *PackageContext) ensureURLSchemeDependency(meta versioning.DependencyMeta) error {
	switch meta.Scheme {
	case "plugin":
		return pcx.ensurePluginDependency(meta)
	case "includes":
		return pcx.ensureIncludesDependency(meta)
	case "filterscript":
		return pcx.ensureFilterscriptDependency(meta)
	default:
		return errors.Errorf("unsupported URL scheme: %s", meta.Scheme)
	}
}

// ensurePluginDependency handles plugin:// scheme dependencies
func (pcx *PackageContext) ensurePluginDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local plugin: plugin://local/path
		pluginPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !util.Exists(pluginPath) {
			return errors.Errorf("local plugin path does not exist: %s", pluginPath)
		}

		pluginMeta := versioning.DependencyMeta{
			Scheme: "plugin",
			Local:  meta.Local,
			User:   "local",
			Repo:   filepath.Base(meta.Local),
		}

		pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
		print.Verb(meta, "added local plugin dependency:", pluginPath)
		return nil
	}

	// Remote plugin: plugin://user/repo or plugin://user/repo:tag
	// Treat as a regular dependency but mark it as a plugin
	remoteMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}

	err := pcx.EnsurePackage(remoteMeta, false)
	if err != nil {
		return err
	}

	pcx.AllPlugins = append(pcx.AllPlugins, remoteMeta)
	print.Verb(meta, "added remote plugin dependency:", remoteMeta)
	return nil
}

// ensureIncludesDependency handles includes:// scheme dependencies
func (pcx *PackageContext) ensureIncludesDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local includes: includes://local/path
		includesPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !util.Exists(includesPath) {
			return errors.Errorf("local includes path does not exist: %s", includesPath)
		}

		pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
		print.Verb(meta, "added local includes path:", includesPath)
		return nil
	}

	// Remote includes: includes://user/repo or includes://user/repo:tag
	// Ensure the dependency and add its path to includes
	remoteMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}

	err := pcx.EnsurePackage(remoteMeta, false)
	if err != nil {
		return err
	}

	includesPath := filepath.Join(pcx.Package.Vendor, remoteMeta.Repo)
	if remoteMeta.Path != "" {
		includesPath = filepath.Join(includesPath, remoteMeta.Path)
	}
	pcx.AllIncludePaths = append(pcx.AllIncludePaths, includesPath)
	print.Verb(meta, "added remote includes path:", includesPath)
	return nil
}

// ensureFilterscriptDependency handles filterscript:// scheme dependencies
func (pcx *PackageContext) ensureFilterscriptDependency(meta versioning.DependencyMeta) error {
	if meta.IsLocalScheme() {
		// Local filterscript: filterscript://local/path
		filterscriptPath := filepath.Join(pcx.Package.LocalPath, meta.Local)
		if !util.Exists(filterscriptPath) {
			return errors.Errorf("local filterscript path does not exist: %s", filterscriptPath)
		}

		print.Verb(meta, "added local filterscript dependency:", filterscriptPath)
		return nil
	}

	// Remote filterscript: filterscript://user/repo or filterscript://user/repo:tag
	// Treat as a regular dependency
	remoteMeta := versioning.DependencyMeta{
		Site:   meta.Site,
		User:   meta.User,
		Repo:   meta.Repo,
		Tag:    meta.Tag,
		Branch: meta.Branch,
		Commit: meta.Commit,
		Path:   meta.Path,
	}

	err := pcx.EnsurePackage(remoteMeta, false)
	if err != nil {
		return err
	}

	print.Verb(meta, "added filterscript dependency:", remoteMeta)
	return nil
}

func (pcx PackageContext) extractResourceDependencies(
	ctx context.Context,
	pkg pawnpackage.Package,
	res resource.Resource,
) (dir string, err error) {
	dir = filepath.Join(pcx.Package.Vendor, res.Path(pkg.Repo))
	print.Verb(pkg, "installing resource-based dependency", res.Name, "to", dir)

	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		err = errors.Wrap(err, "failed to create target directory")
		return
	}

	_, err = runtime.EnsureVersionedPlugin(
		ctx,
		pcx.GitHub,
		pkg.DependencyMeta,
		dir,
		pcx.Platform,
		res.Version,
		pcx.CacheDir,
		false,
		true,
		false,
		pcx.Package.ExtractIgnorePatterns,
	)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure asset")
		return
	}

	return dir, nil
}
