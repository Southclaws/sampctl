package download

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/pawnpackage"
	"github.com/Southclaws/sampctl/util"
)

// GetPackageList gets a list of known packages from the sampctl package service, if the list does
// not exist locally, it is downloaded and cached for future use.
func GetPackageList(cacheDir string) (packages []pawnpackage.Package, err error) {
	packageFile := filepath.Join(cacheDir, "packages.json")

	var update bool

	info, err := os.Stat(packageFile)
	if os.IsNotExist(err) {
		update = true
	} else if time.Since(info.ModTime()) > time.Hour*24*7 {
		// update package list every week
		update = true
	}

	if update {
		// print to stderr so bash doesn't pick it up as an auto-complete result
		fmt.Fprintln(os.Stderr, "updating package list...") //nolint
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

	var packages []pawnpackage.Package
	err = json.NewDecoder(resp.Body).Decode(&packages)
	if err != nil {
		return errors.Wrap(err, "failed to decode package list")
	}

	contents, err := json.MarshalIndent(packages, "", "    ")
	if err != nil {
		return errors.Wrap(err, "failed to encode packages list")
	}

	err = WritePackageCacheFile(cacheDir, contents)
	if err != nil {
		return err
	}

	return
}

func WritePackageCacheFile(cacheDir string, data []byte) error {
	var err error
	if !util.Exists(cacheDir) {
		err = os.MkdirAll(cacheDir, 0700)
		if err != nil {
			err = errors.Wrap(err, "failed to create path to cache directory")
			return err
		}
	}

	runtimesFile := filepath.Join(cacheDir, "compilers.json")
	err = ioutil.WriteFile(runtimesFile, data, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to write package list to file")
	}

	return nil
}
