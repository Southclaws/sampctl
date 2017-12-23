package rook

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Install adds a new dependency to an existing local parent package
func Install(pkg types.Package, target versioning.DependencyString) (err error) {

	// todo: version checks
	exists := false
	for _, dep := range pkg.Dependencies {
		if dep == target {
			exists = true
		}
	}

	if !exists {
		pkg.Dependencies = append(pkg.Dependencies, target)
	} else {
		fmt.Println("target already exists in dependencies")
	}

	err = EnsureDependencies(&pkg)
	if err != nil {
		return
	}

	if pkg.Format == "json" {
		var contents []byte
		contents, err = json.MarshalIndent(pkg, "", "\t")
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = ioutil.WriteFile(filepath.Join(pkg.Local, "pawn.json"), contents, 0755)
		if err != nil {
			return errors.Wrap(err, "failed to write pawn.json")
		}
	} else {
		var contents []byte
		contents, err = yaml.Marshal(pkg)
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = ioutil.WriteFile(filepath.Join(pkg.Local, "pawn.json"), contents, 0755)
		if err != nil {
			return errors.Wrap(err, "failed to write pawn.yaml")
		}
	}

	return
}
