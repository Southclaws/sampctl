package rook

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// EnsureDependenciesCached will recursively visit a parent package dependencies
// in the cache, pulling them if they do not exist yet.
func EnsureDependenciesCached(pkg *types.Package, platform string) (err error) {
	print.Verb(pkg, "building dependency tree and ensuring cached copies")

	if !pkg.Parent {
		return errors.New("package is not a parent package")
	}
	if pkg.LocalPath == "" {
		return errors.New("package has no known local path")
	}
	if !util.Exists(pkg.Vendor) {
		return errors.New("package has no vendor directory")
	}

	var (
		recurse    func(meta versioning.DependencyMeta)
		visited    = make(map[string]bool)
		pluginMeta versioning.DependencyMeta
	)

	visited[pkg.DependencyMeta.Repo] = true

	recurse = func(meta versioning.DependencyMeta) {
		dependencyDir := filepath.Join(pkg.Vendor, meta.Repo)
		if !util.Exists(dependencyDir) {
			print.Verb(pkg, "dependency", meta, "does not exist locally in", pkg.Vendor, "run sampctl package ensure to update dependencies.")
			return
		}

		subPkg, errInner := PackageFromDir(false, dependencyDir, platform, pkg.Vendor)
		if errInner != nil {
			print.Verb(pkg, "not a package:", meta, errInner)
			pkg.AllDependencies = append(pkg.AllDependencies, meta)
			return
		}

		var incPaths []string
		incPaths, errInner = resolveResourcePaths(subPkg, platform)
		if errInner != nil {
			print.Warn(pkg, "Failed to resolve package resource paths:", errInner)
		}
		pkg.AllIncludePaths = append(pkg.AllIncludePaths, incPaths...)

		// only add the package directory if there are no includes in the resources
		if len(incPaths) == 0 {
			pkg.AllDependencies = append(pkg.AllDependencies, meta)
		}

		visited[meta.Repo] = true

		if subPkg.Runtime != nil {
			for _, pluginDepStr := range subPkg.Runtime.Plugins {
				pluginMeta, errInner = pluginDepStr.AsDep()
				pluginMeta.Tag = subPkg.Tag
				print.Verb(pkg, "adding plugin from package runtime", pluginDepStr, "as", pluginMeta)
				if errInner != nil {
					print.Warn(pkg, "invalid plugin dependency string:", pluginDepStr, "in", subPkg, errInner)
					return
				}
				pkg.AllPlugins = append(pkg.AllPlugins, pluginMeta)
			}
		}

		var subPkgDepMeta versioning.DependencyMeta
		for _, subPkgDep := range subPkg.Dependencies {
			subPkgDepMeta, errInner = subPkgDep.Explode()
			if errInner != nil {
				print.Verb(pkg, "invalid dependency string:", subPkgDepMeta, "in", subPkg, errInner)
				continue
			}
			if _, ok := visited[subPkgDepMeta.Repo]; !ok {
				recurse(subPkgDepMeta)
			}
		}
	}

	var meta versioning.DependencyMeta
	for _, dep := range pkg.GetAllDependencies() {
		meta, err = dep.Explode()
		if err != nil {
			print.Verb(pkg, "invalid dependency string:", dep, "in parent package:", err)
			err = nil
			continue
		}
		recurse(meta)
	}

	if pkg.Runtime != nil {
		for _, pluginDepStr := range pkg.Runtime.Plugins {
			pluginMeta, err = pluginDepStr.AsDep()
			if err != nil {
				print.Verb(pkg, "invalid plugin dependency string:", pluginDepStr, "in parent package:", err)
				err = nil
				continue
			}
			pkg.AllPlugins = append(pkg.AllPlugins, pluginMeta)
		}
	}

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
