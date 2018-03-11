package rook

import (
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// PackageFromDir attempts to parse a directory as a Package by looking for a `pawn.json` or
// `pawn.yaml` file and unmarshalling it - additional parameters are required to specify whether or
// not the package is a "parent package" and where the vendor directory is.
func PackageFromDir(parent bool, dir string, vendor string) (pkg types.Package, err error) {
	pkg, err = types.PackageFromDir(dir)
	if err != nil {
		err = errors.Wrap(err, "failed to read package definition")
		return
	}

	pkg.Parent = parent
	pkg.Local = dir

	if vendor == "" {
		pkg.Vendor = filepath.Join(dir, "dependencies")
	} else {
		pkg.Vendor = vendor
	}

	if err = pkg.Validate(); err != nil {
		return
	}

	if pkg.User == "" {
		pkg.User = "<none>"
	}
	if pkg.Repo == "" {
		pkg.Repo = "<local>"
	}

	if parent && len(pkg.Dependencies) > 0 && len(pkg.AllDependencies) == 0 {
		print.Verb(pkg, "resolving dependencies during package load")
		err = ResolveDependencies(&pkg)
		if err != nil {
			print.Verb("failed to resolve dependency tree:", err)
			err = nil // not a breaking error for PackageFromDir
		}
	}

	return
}

// ResolveDependencies is a function for use by parent packages to iterate through their
// `dependencies/` directory discovering packages and getting their dependencies
func ResolveDependencies(pkg *types.Package) (err error) {
	print.Verb(pkg, "resolving dependency tree into a flattened list...")
	if !pkg.Parent {
		return errors.New("package is not a parent package")
	}

	if pkg.Local == "" {
		return errors.New("package has no known local path")
	}

	if !util.Exists(pkg.Vendor) {
		return
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

		pkg.AllDependencies = append(pkg.AllDependencies, meta)
		visited[meta.Repo] = true

		var subPkg types.Package
		subPkg, err = PackageFromDir(false, dependencyDir, pkg.Vendor)
		if err != nil {
			print.Verb(pkg, "not a package:", meta, err)
			return
		}

		if subPkg.Runtime != nil {
			for _, pluginDepStr := range subPkg.Runtime.Plugins {
				pluginMeta, err = pluginDepStr.AsDep()
				if err != nil {
					print.Warn(pkg, "invalid plugin dependency string:", pluginDepStr, "in", subPkg, err)
					return
				}
				pkg.AllPlugins = append(pkg.AllPlugins, pluginMeta)
			}
		}

		for _, res := range subPkg.Resources {
			resPath := filepath.Join(pkg.Vendor, res.Path(subPkg))
			for _, resInc := range res.Includes {
				resIncPath := filepath.Join(resPath, filepath.Dir(resInc))
				if util.Exists(resIncPath) {
					subPkg.AllIncludePaths = append(subPkg.AllIncludePaths, resIncPath)
				}
			}
		}

		var subPkgDepMeta versioning.DependencyMeta
		for _, subPkgDep := range subPkg.Dependencies {
			subPkgDepMeta, err = subPkgDep.Explode()
			if err != nil {
				print.Verb(pkg, "invalid dependency string:", subPkgDepMeta, "in", subPkg, err)
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
