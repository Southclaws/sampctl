package rook

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func EnsureDependenciesCached(
	pkg types.Package,
	platform,
	cacheDir string,
	auth transport.AuthMethod,
) (
	allDependencies []versioning.DependencyMeta,
	allIncludePaths []string,
	allPlugins []versioning.DependencyMeta,
	errOuter error,
) {
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
		globalVendor   = filepath.Join(cacheDir, "packages")
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

			if !util.Exists(dependencyPath) {
				print.Verb(pkg, "cloning fresh copy of", currentMeta, "to package", dependencyPath)

				errInner = os.MkdirAll(dependencyPath, 0700)
				if errInner != nil {
					print.Erro(errInner)
					return
				}

				_, errInner = CloneDependency(currentMeta, dependencyPath, auth)
				if errInner != nil {
					print.Erro(errInner)
					return
				}
			}

			currentPackage, errInner = PackageFromDir(false, dependencyPath, platform, cacheDir, globalVendor, auth)
			if errInner != nil {
				print.Verb(pkg, "dependency", currentMeta, "is not a package:", errInner)
				return
			}
		}

		print.Verb(pkg, "Resolving resource paths")
		var incPaths []string
		incPaths, errInner = resolveResourcePaths(currentPackage, platform)
		if errInner != nil {
			print.Warn(pkg, "Failed to resolve package resource paths:", errInner)
		}
		allIncludePaths = append(allIncludePaths, incPaths...)
		if len(incPaths) == 0 && !firstIter {
			// only add the package as a full dependency if there are no
			// includes in the resources and this isn't the first iteration

			// TODO: this should be handled later on down the line, this is
			// semantically incorrect because the currentMeta IS still a "dependency"
			// it just doesn't want its path to be in the includes path that is
			// passed to the compiler. There needs to be some way to store that
			// information at this point and then use it at a later time to omit
			// the package from the compiler paths list.
			print.Verb(pkg, "adding", currentMeta, "to resultant dependencies list")
			allDependencies = append(allDependencies, currentMeta)
		} else {
			print.Verb(pkg, "not adding", currentMeta, "because it has includes within resources:", incPaths)
		}

		// mark the repo as visited so we don't hit it again in case it appears
		// multiple times within the dependency tree.
		visited[currentMeta.Repo] = true

		// now iterate the subpackage
		if currentPackage.Runtime != nil {
			for _, pluginDepStr := range currentPackage.Runtime.Plugins {
				pluginMeta, errInner = pluginDepStr.AsDep()
				pluginMeta.Tag = currentPackage.Tag
				print.Verb(pkg, "adding plugin from package runtime", pluginDepStr, "as", pluginMeta)
				if errInner != nil {
					print.Warn(pkg, "invalid plugin dependency string:", pluginDepStr, "in", currentPackage, errInner)
					return
				}
				allPlugins = append(allPlugins, pluginMeta)
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

		var subPackageDepMeta versioning.DependencyMeta
		for _, subPackageDepString := range subPackageDepStrings {
			subPackageDepMeta, errInner = subPackageDepString.Explode()
			if errInner != nil {
				print.Verb(pkg, "invalid dependency string:", subPackageDepMeta, "in", currentPackage, errInner)
				continue
			}
			if _, ok := visited[subPackageDepMeta.Repo]; !ok {
				recurse(subPackageDepMeta)
			}
		}
	}
	recurse(pkg.DependencyMeta)

	return
}

func resolveResourcePaths(pkg types.Package, platform string) (paths []string, err error) {
	for _, res := range pkg.Resources {
		if res.Platform != platform {
			print.Verb(pkg, "ignoring platform mismatch", res.Platform)
			continue
		}

		targetPath := filepath.Join(pkg.Vendor, res.Path(pkg))

		if len(res.Includes) > 0 {
			var info os.FileInfo
			info, err = os.Stat(targetPath)
			if err != nil {
				err = errors.Wrapf(err, "failed to stat target path %s", targetPath)
				return
			}
			if info.IsDir() {
				print.Verb(pkg, "adding resource include path", targetPath)
				paths = append(paths, targetPath)
			}
		}
	}
	return
}

// CloneDependency clones a package to path using the default branch
func CloneDependency(meta versioning.DependencyMeta, path string, auth transport.AuthMethod) (repo *git.Repository, err error) {
	print.Verb(meta, "cloning dependency package")

	cloneOpts := &git.CloneOptions{
		URL:   meta.URL(),
		Depth: 1000,
	}

	if meta.SSH != "" {
		cloneOpts.Auth = auth
	}

	repo, err = git.PlainClone(path, false, cloneOpts)
	return
}
