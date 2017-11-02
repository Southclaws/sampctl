package rook

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// PackageFromDir attempts to parse a directory as a Package by looking for a `pawn.json` or
// `pawn.yaml` file and unmarshalling it.
func PackageFromDir(dir string) (pkg Package, err error) {
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

	pkg.local = dir

	if err = pkg.Validate(); err != nil {
		return
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
		err = errors.Wrap(err, "failed to read pawn.json")
		return
	}

	err = yaml.Unmarshal(contents, &pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to unmarshal pawn.json")
		return
	}

	return
}
