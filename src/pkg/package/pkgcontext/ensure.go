package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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

// EnsureDependencies traverses package dependencies and ensures they are up to date
func (pcx *PackageContext) EnsureDependencies(ctx context.Context, forceUpdate bool) (err error) {
	return pcx.EnsureDependenciesWithRuntime(ctx, forceUpdate, true)
}

// EnsureDependenciesWithRuntime traverses package dependencies and ensures they are up to date,
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

// ensureDependencyRepository makes sure the dependency repository exists and is accessible
func (pcx *PackageContext) ensureDependencyRepository(meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) (*git.Repository, error) {
	repo, err := git.PlainOpen(dependencyPath)
	if err == nil {
		// Repository exists, validate it
		head, headErr := repo.Head()
		if headErr != nil {
			print.Verb(meta, "existing repository has invalid HEAD, re-cloning")
			return pcx.recloneDependency(meta, dependencyPath)
		}
		print.Verb(meta, "repository already exists at", head.Hash().String()[:8])
		return repo, nil
	}

	if err != git.ErrRepositoryNotExists {
		// Unexpected error - try to recover
		print.Verb(meta, "error opening repository:", err)
		return pcx.recloneDependency(meta, dependencyPath)
	}

	// Repository doesn't exist - clone from cache
	print.Verb(meta, "repository does not exist, cloning from cache")
	return pcx.cloneDependencyFromCache(meta, dependencyPath)
}

// cloneDependencyFromCache clones a dependency from the cache
func (pcx *PackageContext) cloneDependencyFromCache(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	repo, err := pcx.EnsureDependencyFromCache(meta, dependencyPath, false)
	if err != nil {
		// Failed to clone from cache - cleanup and report
		print.Verb(meta, "failed to clone from cache:", err)
		os.RemoveAll(dependencyPath)
		return nil, errors.Wrap(err, "failed to clone dependency from cache")
	}

	// Validate the cloned repository
	valid, validationErr := ValidateRepository(dependencyPath)
	if validationErr != nil || !valid {
		print.Verb(meta, "cloned repository failed validation")
		os.RemoveAll(dependencyPath)
		if validationErr != nil {
			return nil, errors.Wrap(validationErr, "cloned repository is invalid")
		}
		return nil, errors.New("cloned repository failed validation")
	}

	return repo, nil
}

// recloneDependency removes and re-clones a dependency
func (pcx *PackageContext) recloneDependency(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	print.Verb(meta, "re-cloning dependency at", dependencyPath)

	err := os.RemoveAll(dependencyPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to remove corrupted dependency")
	}

	return pcx.cloneDependencyFromCache(meta, dependencyPath)
}

// updateRepoStateWithRecovery updates repository with automatic recovery on failure
func (pcx *PackageContext) updateRepoStateWithRecovery(repo *git.Repository, meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) error {
	err := pcx.updateRepoState(repo, meta, forceUpdate)
	if err == nil {
		return nil
	}

	print.Verb(meta, "first update attempt failed:", err)

	repairErr := RepairRepository(dependencyPath)
	if repairErr == nil {
		print.Verb(meta, "repository repaired, retrying update")
		repo, openErr := git.PlainOpen(dependencyPath)
		if openErr == nil {
			err = pcx.updateRepoState(repo, meta, true)
			if err == nil {
				return nil
			}
		}
	}

	// Repair failed or update still fails - try force update
	print.Verb(meta, "attempting force update")
	err = pcx.updateRepoState(repo, meta, true)
	if err == nil {
		return nil
	}

	print.Verb(meta, "all update attempts failed, re-cloning dependency")
	_, err = pcx.recloneDependency(meta, dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to recover by re-cloning")
	}

	repo, err = git.PlainOpen(dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to open re-cloned repository")
	}

	return pcx.updateRepoState(repo, meta, forceUpdate)
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

	err := pcx.ensureRegularPackage(remoteMeta, false)
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

	err := pcx.ensureRegularPackage(remoteMeta, false)
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

	err := pcx.ensureRegularPackage(remoteMeta, false)
	if err != nil {
		return err
	}

	print.Verb(meta, "added filterscript dependency:", remoteMeta)
	return nil
}

// ensureRegularPackage handles regular Git-based dependencies (extracted from original EnsurePackage logic)
func (pcx *PackageContext) ensureRegularPackage(meta versioning.DependencyMeta, forceUpdate bool) error {
	var (
		dependencyPath = filepath.Join(pcx.Package.Vendor, meta.Repo)
		needToClone    = false // do we need to clone a new repo?
		head           *plumbing.Reference
	)

	repo, err := git.PlainOpen(dependencyPath)
	if err != nil && err != git.ErrRepositoryNotExists {
		return errors.Wrap(err, "failed to open dependency repository")
	} else if err == git.ErrRepositoryNotExists {
		print.Verb(meta, "package does not exist at", dependencyPath, "cloning new copy")
		needToClone = true
	} else {
		head, err = repo.Head()
		if err != nil {
			print.Verb(meta, "package already exists but failed to get repository HEAD:", err)
			needToClone = true
			err = os.RemoveAll(dependencyPath)
			if err != nil {
				return errors.Wrap(err, "failed to temporarily remove possibly corrupted dependency repo")
			}
		} else {
			print.Verb(meta, "package already exists at", head)
		}
	}

	if needToClone {
		print.Verb(meta, "need to clone new copy from cache")
		repo, err = pcx.EnsureDependencyFromCache(meta, dependencyPath, false)
		if err != nil {
			errInner := os.RemoveAll(dependencyPath)
			if errInner != nil {
				return errors.Wrap(errInner, "failed to remove corrupted dependency repo")
			}

			errInner = errors.Wrap(err, "failed to ensure dependency from cache")
			if errInner != nil {
				return errInner
			}
			return nil
		}
	}

	print.Verb(meta, "updating dependency package")
	err = pcx.updateRepoState(repo, meta, forceUpdate)
	if err != nil {
		// try once more, but force a pull
		print.Verb(meta, "unable to update repo in given state, force-pulling latest from repo tip")
		err = pcx.updateRepoState(repo, meta, true)
		if err != nil {
			return errors.Wrap(err, "failed to update repo state")
		}
	}

	// To install resources (includes from within release archives) we can't use the user's locally
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

// updateRepoState takes a repo that exists on disk and ensures it matches tag, branch or commit constraints
func (pcx *PackageContext) updateRepoState(
	repo *git.Repository,
	meta versioning.DependencyMeta,
	forcePull bool,
) (err error) {
	print.Verb(meta, "updating repository state with", pcx.GitAuth, "authentication method")

	var wt *git.Worktree
	if forcePull {
		print.Verb(meta, "performing forced pull to latest tip")
		repo, err = pcx.EnsureDependencyFromCache(meta, filepath.Join(pcx.Package.Vendor, meta.Repo), true)
		if err != nil {
			return errors.Wrap(err, "failed to ensure dependency in cache")
		}
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}

		err = wt.Pull(&git.PullOptions{
			Depth: 1000, // TODO: We might want to consider removing depth for better reliability, or add a configurable option
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to force pull for full update")
		}
	} else {
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}
	}

	var (
		ref      *plumbing.Reference
		pullOpts = &git.PullOptions{}
	)

	if meta.SSH != "" {
		pullOpts.Auth = pcx.GitAuth
	}

	if meta.Tag != "" {
		print.Verb(meta, "package has tag constraint:", meta.Tag)

		ref, err = versioning.RefFromTag(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from tag")
		}
	} else if meta.Branch != "" {
		print.Verb(meta, "package has branch constraint:", meta.Branch)

		pullOpts.Depth = 1000 // get full history
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + meta.Branch)

		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo branch")
		}

		ref, err = versioning.RefFromBranch(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from branch")
		}
	} else if meta.Commit != "" {
		pullOpts.Depth = 1000 // get full history

		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo")
		}

		ref, err = versioning.RefFromCommit(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from commit")
		}
	}

	if ref != nil {
		print.Verb(meta, "checking out ref determined from constraint:", ref)

		err = wt.Checkout(&git.CheckoutOptions{
			Hash:  ref.Hash(),
			Force: true,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
		}
		print.Verb(meta, "successfully checked out to", ref.Hash())
	} else {
		print.Verb(meta, "package does not have version constraint pulling latest")

		err = wt.Pull(pullOpts)
		if err != nil {
			if err == git.NoErrAlreadyUpToDate {
				err = nil
			} else {
				return errors.Wrap(err, "failed to fetch latest package")
			}
		}
	}

	return err
}
