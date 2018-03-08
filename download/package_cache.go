package download

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// GetPackageList gets a list of known packages from the sampctl package service, if the list does
// not exist locally, it is downloaded and cached for future use.
func GetPackageList(cacheDir string) (packages []types.Package, err error) {
	packageFile := filepath.Join(cacheDir, "packages.json")

	if !util.Exists(packageFile) {
		err = UpdatePackageList(cacheDir)
		if err != nil {
			return
		}
	}

	contents, err := ioutil.ReadFile(packageFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read package cache file")
	}

	err = json.Unmarshal(contents, &packages)
	return
}

// UpdatePackageList downloads a list of all packages to a file in the cache directory
func UpdatePackageList(cacheDir string) (err error) {
	resp, err := http.Get("http://list.packages.sampctl.com")
	if err != nil {
		return errors.Wrap(err, "failed to download package list")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("package list status %s", resp.Status)
	}

	var packages []types.Package
	err = json.NewDecoder(resp.Body).Decode(&packages)
	if err != nil {
		return errors.Wrap(err, "failed to decode package list")
	}

	contents, err := json.Marshal(packages)
	if err != nil {
		return errors.Wrap(err, "failed to encode packages list")
	}

	packageFile := filepath.Join(cacheDir, "packages.json")
	err = ioutil.WriteFile(packageFile, contents, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to write package list to file")
	}

	return
}
