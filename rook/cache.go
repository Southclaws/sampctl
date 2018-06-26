package rook

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func (pcx *PackageContext) EnsureDependenciesCached() (errOuter error) {
	print.Verb(pcx.Package, "building dependency tree and ensuring cached copies")

	if !pcx.Package.Parent {
		errOuter = errors.New("package is not a parent package")
		return
	}
	if pcx.Package.LocalPath == "" {
		errOuter = errors.New("package has no known local path")
		return
	}

	// This recursive operation requires quite a lot of state! There is probably
	// a better method to break this up but so far, this has worked fine.
	var (
		recurse        func(meta versioning.DependencyMeta)
		visited        = make(map[string]bool)
		dependencyPath = pcx.Package.LocalPath
		firstIter      = true
		currentPackage types.Package
		errInner       error
	)

	// clear the dependencies list in case this function is being called on an
	// already initialised context that already has some dependencies listed.
	pcx.AllDependencies = nil

	// set the parent package visited state to true, just in case it depends on
	// itself or a dependency depends on it. This should never happen but if it
	// does, this prevents an infinite recursion.
	visited[pcx.Package.DependencyMeta.Repo] = true

	recurse = func(currentMeta versioning.DependencyMeta) {
		// the first iteration of this recursive function is called on the
		// parent package. This means it does not need to be cloned to the cache
		// and the path will be it's true, user-defined location.
		if firstIter {
			print.Verb(pcx.Package, "processing parent package in recursive function")
			currentPackage = pcx.Package // set the current package to the parent
		} else {
			dependencyPath = currentMeta.CachePath(pcx.CacheDir)

			print.Verb(pcx.Package, "ensuring", currentMeta, "to", dependencyPath)

			_, errInner = pcx.EnsureDependencyCached(currentMeta, false)
			if errInner != nil {
				print.Erro(errInner)
				return
			}

			currentPackage, errInner = types.PackageFromDir(dependencyPath)
			if errInner != nil {
				print.Verb(pcx.Package, "dependency", currentMeta, "is not a package:", errInner)
				return
			}
		}

		// mark the repo as visited so we don't hit it again in case it appears
		// multiple times within the dependency tree.
		visited[currentMeta.Repo] = true

		var subPackageDepStrings []versioning.DependencyString

		if currentPackage.Parent {
			subPackageDepStrings = currentPackage.GetAllDependencies()
		} else {
			subPackageDepStrings = currentPackage.Dependencies
		}

		// first iteration has finished, mark it false and next iterations will
		// operate on dependencies
		firstIter = false

		print.Verb(pcx.Package, "recursively iterating", len(subPackageDepStrings), "dependencies of", currentPackage)
		var subPackageDepMeta versioning.DependencyMeta
		for _, subPackageDepString := range subPackageDepStrings {
			subPackageDepMeta, errInner = subPackageDepString.Explode()
			if errInner != nil {
				print.Verb(pcx.Package, "invalid dependency string:", subPackageDepMeta, "in", currentPackage, errInner)
				continue
			}
			if _, ok := visited[subPackageDepMeta.Repo]; !ok {
				pcx.AllDependencies = append(pcx.AllDependencies, subPackageDepMeta)
				recurse(subPackageDepMeta)
			} else {
				print.Verb(pcx.Package, "already visited", subPackageDepMeta)
			}
		}
	}
	recurse(pcx.Package.DependencyMeta)

	return
}

// EnsureDependencyFromCache ensures the repository at `path` is up to date
func (pcx PackageContext) EnsureDependencyFromCache(meta versioning.DependencyMeta, path string, forceUpdate bool) (repo *git.Repository, err error) {
	print.Verb(meta, "ensuring dependency package from cache to", path, "force update:", forceUpdate)

	from, err := filepath.Abs(meta.CachePath(pcx.CacheDir))
	if err != nil {
		err = errors.Wrap(err, "failed to make canonical path to cached copy")
		return
	}
	if !util.Exists(filepath.Join(from, ".git")) || forceUpdate {
		_, err = pcx.EnsureDependencyCached(meta, forceUpdate)
		if err != nil {
			return
		}
	}

	repo, err = pcx.ensureRepoExists(from, path, meta.Branch, meta.SSH != "", forceUpdate)
	return
}

// EnsureDependencyCached clones a package to path using the default branch
func (pcx PackageContext) EnsureDependencyCached(meta versioning.DependencyMeta, forceUpdate bool) (repo *git.Repository, err error) {
	print.Verb(meta, "ensuring dependency package is cached, force update:", forceUpdate)
	return pcx.ensureRepoExists(meta.URL(), meta.CachePath(pcx.CacheDir), meta.Branch, meta.SSH != "", forceUpdate)
}

func (pcx PackageContext) ensureRepoExists(from, to, branch string, ssh, forceUpdate bool) (repo *git.Repository, err error) {
	repo, err = git.PlainOpen(to)
	if err != nil {
		print.Verb("no repo at", to, "-", err, "cloning new copy")
		if util.Exists(to) {
			print.Verb("removing existing folder", to)
			err = os.RemoveAll(to)
			if err != nil {
				return
			}
		}

		err = os.MkdirAll(to, 0700)
		if err != nil {
			return
		}

		cloneOpts := &git.CloneOptions{
			URL:   from,
			Depth: 1000,
		}
		if branch != "" {
			cloneOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + branch)
		}

		if ssh {
			cloneOpts.Auth = pcx.GitAuth
		}

		print.Verb("cloning latest copy to", to, "with", cloneOpts)
		return git.PlainClone(to, false, cloneOpts)
	}

	if forceUpdate {
		var wt *git.Worktree
		wt, err = repo.Worktree()
		if err != nil {
			return
		}

		pullOpts := &git.PullOptions{
			Depth: 1000,
		}
		if branch != "" {
			pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + branch)
		}

		print.Verb("pulling latest copy to", to, "with", pullOpts)
		err = wt.Pull(pullOpts)
		if err != nil && err != git.NoErrAlreadyUpToDate {
			print.Verb("failed to pull, removing repository and starting fresh")
			err = os.RemoveAll(to)
			if err != nil {
				err = errors.Wrap(err, "failed to remove repo in bad state for re-clone")
				return
			}
			return pcx.ensureRepoExists(from, to, branch, ssh, false)
		}
	}

	return repo, nil
}
