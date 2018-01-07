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
		err = ResolveDependencies(&pkg)
		if err != nil {
			print.Warn("failed to resolve dependency tree:", err)
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

	depsDir := filepath.Join(pkg.Local, "dependencies")

	if !util.Exists(depsDir) {
		return
	}

	var recurse func(dependencyString versioning.DependencyString)
	var pluginMeta versioning.DependencyMeta

	recurse = func(dependencyString versioning.DependencyString) {
		dependencyMeta, err := dependencyString.Explode()
		if err != nil {
			print.Verb(pkg, "invalid dependency string:", dependencyString)
			return
		}

		dependencyDir := filepath.Join(depsDir, dependencyMeta.Repo)
		if !util.Exists(dependencyDir) {
			print.Verb(pkg, "dependency", dependencyString, "does not exist locally in", depsDir, "run sampctl package ensure to update dependencies.")
			return
		}

		pkg.AllDependencies = append(pkg.AllDependencies, dependencyMeta)

		subPkg, err := PackageFromDir(false, dependencyDir, depsDir)
		if err != nil {
			print.Verb(pkg, "not a package:", dependencyString, err)
			return
		}

		if subPkg.Runtime != nil {
			for _, pluginDepStr := range subPkg.Runtime.Plugins {
				pluginMeta, err = pluginDepStr.AsDep()
				if err != nil {
					print.Verb(pkg, "invalid plugin dependency string:", pluginDepStr)
					return
				}
				pkg.AllPlugins = append(pkg.AllPlugins, pluginMeta)
			}
		}

		for _, depStr := range subPkg.Dependencies {
			recurse(depStr)
		}
	}

	for _, depStr := range pkg.GetAllDependencies() {
		recurse(depStr)
	}

	if pkg.Runtime != nil {
		for _, pluginDepStr := range pkg.Runtime.Plugins {
			pluginMeta, err = pluginDepStr.AsDep()
			if err != nil {
				print.Erro(pkg, "invalid plugin dependency string:", pluginDepStr)
				return
			}
			pkg.AllPlugins = append(pkg.AllPlugins, pluginMeta)
		}
	}

	return
}
