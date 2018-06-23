package rook

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func (pcx *PackageContext) EnsureDependenciesCached() (errOuter error) {
	pkg := pcx.Package

	print.Verb(pkg, "building dependency tree and ensuring cached copies")

	if !pkg.Parent {
		errOuter = errors.New("package is not a parent package")
		return
	}
	if pkg.LocalPath == "" {
		errOuter = errors.New("package has no known local path")
		return
	}

	// This recursive operation requires quite a lot of state! There is probably
	// a better method to break this up but so far, this has worked fine.
	var (
		recurse        func(meta versioning.DependencyMeta)
		globalVendor   = filepath.Join(pcx.CacheDir, "packages")
		visited        = make(map[string]bool)
		pluginMeta     versioning.DependencyMeta
		dependencyPath = pkg.LocalPath
		firstIter      = true
		currentPackage types.Package
		errInner       error
	)

	// set the parent package visited state to true, just in case it depends on
	// itself or a dependency depends on it. This should never happen but if it
	// does, this prevents an infinite recursion.
	visited[pkg.DependencyMeta.Repo] = true

	recurse = func(currentMeta versioning.DependencyMeta) {
		// the first iteration of this recursive function is called on the
		// parent package. This means it does not need to be cloned to the cache
		// and the path will be it's true, user-defined location.
		if firstIter {
			print.Verb(pkg, "processing parent package in recursive function")
			currentPackage = pkg // set the current package to the parent
		} else {
			dependencyPath = filepath.Join(globalVendor, currentMeta.Repo)

			print.Verb(pkg, "ensuring cached copy of", currentMeta)

			_, errInner = pcx.EnsureDependencyCached(currentMeta)
			if errInner != nil {
				print.Erro(errInner)
				return
			}

			currentPackage, errInner = types.PackageFromDir(dependencyPath)
			if errInner != nil {
				print.Verb(pkg, "dependency", currentMeta, "is not a package:", errInner)
				return
			}
			pcx.AllDependencies = append(pcx.AllDependencies, currentMeta)
		}

		// mark the repo as visited so we don't hit it again in case it appears
		// multiple times within the dependency tree.
		visited[currentMeta.Repo] = true

		// now iterate the runtime plugins of the package. If there are entries
		// in here, that means this package is actually a plugin package that
		// provides binaries that should be downloaded.
		if currentPackage.Runtime != nil {
			print.Verb(pkg, "gathering plugins from", currentPackage)
			for _, pluginDepStr := range currentPackage.Runtime.Plugins {
				pluginMeta, errInner = pluginDepStr.AsDep()
				pluginMeta.Tag = currentPackage.Tag
				print.Verb(pkg, "adding plugin from package runtime", pluginDepStr, "as", pluginMeta)
				if errInner != nil {
					print.Warn(pkg, "Invalid plugin dependency string:", pluginDepStr, "in", currentPackage, errInner)
					return
				}

				_, resource, err := runtime.EnsureVersionedPluginCached(context.Background(), pluginMeta, pcx.Platform, pcx.CacheDir, false, pcx.GitHub)
				if err != nil {
					print.Warn(pkg, "Failed to download dependency plugin", pluginMeta, "to cache")
					return
				}
				pcx.AllResources = append(pcx.AllResources, resource)
				pcx.AllPlugins = append(pcx.AllPlugins, pluginMeta)
			}
		}

		var subPackageDepStrings []versioning.DependencyString

		if currentPackage.Parent {
			subPackageDepStrings = currentPackage.GetAllDependencies()
		} else {
			subPackageDepStrings = currentPackage.Dependencies
		}

		// first iteration has finished, mark it false and next iterations will
		// operate on dependencies
		firstIter = false

		print.Verb(pkg, "recursively iterating", len(subPackageDepStrings), "dependencies of", currentPackage)
		var subPackageDepMeta versioning.DependencyMeta
		for _, subPackageDepString := range subPackageDepStrings {
			subPackageDepMeta, errInner = subPackageDepString.Explode()
			if errInner != nil {
				print.Verb(pkg, "invalid dependency string:", subPackageDepMeta, "in", currentPackage, errInner)
				continue
			}
			if _, ok := visited[subPackageDepMeta.Repo]; !ok {
				print.Verb(pkg, "recursing over", subPackageDepMeta)
				recurse(subPackageDepMeta)
			} else {
				print.Verb(pkg, "already visited", subPackageDepMeta)
			}
		}
	}
	recurse(pkg.DependencyMeta)

	return
}

// EnsureDependencyCached clones a package to path using the default branch
func (pcx PackageContext) EnsureDependencyCached(meta versioning.DependencyMeta) (repo *git.Repository, err error) {
	print.Verb(meta, "cloning dependency package")
	return pcx.cloneDependency(meta.URL(), filepath.Join(pcx.CacheDir, "packages", meta.Repo), meta.SSH != "")
}

// EnsureDependencyFromCache ensures the repository at `path` is up to date
func (pcx PackageContext) EnsureDependencyFromCache(meta versioning.DependencyMeta, path string) (repo *git.Repository, err error) {
	print.Verb(meta, "ensuring dependency package")

	from := filepath.Join(pcx.CacheDir, "packages", meta.Repo)
	if !util.Exists(from) {
		_, err = pcx.EnsureDependencyCached(meta)
		if err != nil {
			return
		}
	}

	repo, err = pcx.cloneDependency(from, path, meta.SSH != "")
	return
}

func (pcx PackageContext) cloneDependency(from, to string, ssh bool) (repo *git.Repository, err error) {
	repo, err = git.PlainOpen(to)
	if err != nil {
		if util.Exists(to) {
			err = os.RemoveAll(to)
			if err != nil {
				return
			}
		}

		err = os.MkdirAll(to, 0700)
		if err != nil {
			print.Erro(err)
			return
		}

		cloneOpts := &git.CloneOptions{
			URL:   from,
			Depth: 1000,
		}

		if ssh {
			cloneOpts.Auth = pcx.GitAuth
		}

		return git.PlainClone(to, false, cloneOpts)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return
	}

	err = wt.Pull(&git.PullOptions{
		Depth: 1000,
	})
	if err != nil {
		return
	}

	return
}
