package rook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/Southclaws/sampctl/util"
)

// PackageFromDir attempts to parse a directory as a Package by looking for a `pawn.json` or
// `pawn.yaml` file and unmarshalling it - additional parameters are required to specify whether or
// not the package is a "parent package" and where the vendor directory is.
func PackageFromDir(parent bool, dir string, vendor string) (pkg Package, err error) {
	jsonFile := filepath.Join(dir, "pawn.json")
	if util.Exists(jsonFile) {
		pkg, err = PackageFromJSON(jsonFile)
	} else {
		yamlFile := filepath.Join(dir, "pawn.yaml")
		if util.Exists(yamlFile) {
			pkg, err = PackageFromYAML(yamlFile)
		} else {
			err = errors.New("directory does not contain a pawn.json or pawn.yaml file")
		}
	}
	if err != nil {
		return
	}

	pkg.parent = parent
	pkg.local = dir

	if vendor == "" {
		pkg.vendor = filepath.Join(dir, "dependencies")
	} else {
		pkg.vendor = vendor
	}

	if err = pkg.Validate(); err != nil {
		return
	}

	if parent {
		err = pkg.ResolveDependencies()
		if err != nil {
			err = errors.Wrap(err, "failed to resolve all dependencies")
			return
		}
	}

	return
}

// PackageFromJSON creates a config from a JSON file
func PackageFromJSON(file string) (pkg Package, err error) {
	var contents []byte
	contents, err = ioutil.ReadFile(file)
	if err != nil {
		err = errors.Wrap(err, "failed to read pawn.json")
		return
	}

	err = json.Unmarshal(contents, &pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal pawn.json")
		return
	}

	return
}

// PackageFromYAML creates a config from a YAML file
func PackageFromYAML(file string) (pkg Package, err error) {
	var contents []byte
	contents, err = ioutil.ReadFile(file)
	if err != nil {
		err = errors.Wrap(err, "failed to read pawn.yaml")
		return
	}

	err = yaml.Unmarshal(contents, &pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal pawn.yaml")
		return
	}

	return
}

// ResolveDependencies is a function for use by parent packages to iterate through their
// `dependencies/` directory discovering packages and getting their dependencies
func (pkg *Package) ResolveDependencies() (err error) {
	if !pkg.parent {
		return errors.New("package is not a parent package")
	}

	if pkg.local == "" {
		return errors.New("package has no known local path")
	}

	depsDir := filepath.Join(pkg.local, "dependencies")

	if !util.Exists(depsDir) {
		fmt.Println("dependencies directory does not exist, run sampctl package ensure to update dependencies")
		return
	}

	for _, dependencyString := range pkg.Dependencies {
		dependencyMeta, err := dependencyString.Explode()
		if err != nil {
			fmt.Println(pkg, "invalid dependency string:", dependencyString)
			continue
		}

		dependencyDir := filepath.Join(depsDir, dependencyMeta.Repo)
		if !util.Exists(dependencyDir) {
			fmt.Println(pkg, "dependency", dependencyString, "does not exist locally in", depsDir, "run sampctl package ensure to update dependencies.")
			continue
		}

		pkg.allDependencies = append(pkg.allDependencies, dependencyMeta)

		subPkg, err := PackageFromDir(false, dependencyDir, depsDir)
		if err != nil {
			fmt.Println(pkg, "dependency is not a Pawn package:", dependencyString, err)
			continue
		}

		for _, depStr := range subPkg.Dependencies {
			depMeta, err := depStr.Explode()
			if err != nil {
				fmt.Println(pkg, "dependency, ", dependencyString, "has an invalid dependency:", depStr)
				continue
			}
			pkg.allDependencies = append(pkg.allDependencies, depMeta)
		}
	}

	return
}
