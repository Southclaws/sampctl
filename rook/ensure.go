package rook

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// ErrNotRemotePackage describes a repository that does not contain a package definition file
var ErrNotRemotePackage = errors.New("remote repository does not declare a package")

// EnsureDependencies traverses package dependencies and ensures they are up to date
func (pcx *PackageContext) EnsureDependencies(ctx context.Context, forceUpdate bool) (err error) {
	if pcx.Package.LocalPath == "" {
		return errors.New("package does not represent a locally stored package")
	}

	if !util.Exists(pcx.Package.LocalPath) {
		return errors.New("package local path does not exist")
	}

	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")

	for _, dependency := range pcx.AllDependencies {
		errInner := pcx.EnsurePackage(dependency, forceUpdate)
		if errInner != nil {
			print.Warn(errors.Wrapf(errInner, "failed to ensure package %s", dependency))
			continue
		}
		print.Info(pcx.Package, "successfully ensured dependency files for", dependency)
	}

	if pcx.Package.Local {
		print.Verb(pcx.Package, "package is local, ensuring binaries too")
		pcx.ActualRuntime.WorkingDir = pcx.Package.LocalPath
		pcx.ActualRuntime.Format = pcx.Package.Format

		pcx.ActualRuntime.PluginDeps, err = pcx.GatherPlugins()
		if err != nil {
			return
		}
		err = runtime.Ensure(ctx, pcx.GitHub, &pcx.ActualRuntime, false)
		if err != nil {
			return
		}
	}

	return
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
func (pcx *PackageContext) EnsurePackage(meta versioning.DependencyMeta, forceUpdate bool) (err error) {
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
			return errors.Wrap(err, "failed to ensure dependency from cache")
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
	// resources can be downloaded from. Therefore, we instead instantiate a new types.Package from
	// the cached version of the package because the cached copy is always at the latest version, or
	// at least guaranteed to be either later or equal to the local dependency version.
	pkg, err := types.GetCachedPackage(meta, pcx.CacheDir)
	if err != nil {
		return
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
				return
			}
			pcx.AllIncludePaths = append(pcx.AllIncludePaths, includePath)
		}
	}

	return
}

func (pcx PackageContext) extractResourceDependencies(ctx context.Context, pkg types.Package, res types.Resource) (dir string, err error) {
	dir = filepath.Join(pcx.Package.Vendor, res.Path(pkg))
	print.Verb(pkg, "installing resource-based dependency", res.Name, "to", dir)

	err = os.MkdirAll(dir, 0700)
	if err != nil {
		err = errors.Wrap(err, "failed to create target directory")
		return
	}

	_, err = runtime.EnsureVersionedPlugin(ctx, pcx.GitHub, pkg.DependencyMeta, dir, pcx.Platform, pcx.CacheDir, false, true, false)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure asset")
		return
	}

	return
}

// updateRepoState takes a repo that exists on disk and ensures it matches tag, branch or commit constraints
func (pcx *PackageContext) updateRepoState(repo *git.Repository, meta versioning.DependencyMeta, forcePull bool) (err error) {
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
			Depth: 1000, // get full history
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

	return
}
