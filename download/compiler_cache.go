package download

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Southclaws/sampctl/types"
	"github.com/pkg/errors"
)

// GetCompilerList gets a list of known compiler packages from the sampctl repo, if the list does not
// exist locally, it is downloaded and cached for future use.
func GetCompilerList(cacheDir string) (compilers types.Compilers, err error) {
	runtimesFile := filepath.Join(cacheDir, "compilers.json")

	var update bool

	info, err := os.Stat(runtimesFile)
	if os.IsNotExist(err) {
		update = true
	} else if time.Since(info.ModTime()) > time.Hour*24*7 {
		// update package list every week
		update = true
	}

	if update {
		// print to stderr so bash doesn't pick it up as an auto-complete result
		fmt.Fprintln(os.Stderr, "updating compiler list...") // nolint:gas
		err = UpdateCompilerList(cacheDir)
		if err != nil {
			return
		}
	}

	contents, err := ioutil.ReadFile(runtimesFile)
	if err != nil {
		err = errors.Wrap(err, "failed to read package cache file")
		return
	}

	err = json.Unmarshal(contents, &compilers)
	return
}

// UpdateCompilerList downloads a list of all runtime packages to a file in the cache directory
func UpdateCompilerList(cacheDir string) (err error) {
	resp, err := http.Get("https://raw.githubusercontent.com/sampctl/compilers/master/compilers.json")
	if err != nil {
		return errors.Wrap(err, "failed to download package list")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("package list status %s", resp.Status)
	}

	var compilers types.Compilers
	err = json.NewDecoder(resp.Body).Decode(&compilers)
	if err != nil {
		return errors.Wrap(err, "failed to decode package list")
	}

	contents, err := json.MarshalIndent(compilers, "", "    ")
	if err != nil {
		return errors.Wrap(err, "failed to encode compilers list")
	}

	runtimesFile := filepath.Join(cacheDir, "compilers.json")
	err = ioutil.WriteFile(runtimesFile, contents, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to write package list to file")
	}

	return
}
