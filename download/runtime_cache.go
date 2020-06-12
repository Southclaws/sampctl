//nolint:dupl,golint
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
)

// Runtimes is a collection of Package objects for sorting
type Runtimes struct {
	Aliases  map[string]string `json:"aliases"`
	Packages []RuntimePackage  `json:"packages"`
}

// RuntimePackage represents a SA:MP server version, it stores both platform filenames and a checksum
type RuntimePackage struct {
	Version       string            `json:"version"`
	Linux         string            `json:"linux"`
	Win32         string            `json:"win32"`
	LinuxChecksum string            `json:"linux_checksum"`
	Win32Checksum string            `json:"win32_checksum"`
	LinuxPaths    map[string]string `json:"linux_paths"`
	Win32Paths    map[string]string `json:"win32_paths"`
}

// GetRuntimeList gets a list of known runtime packages from the sampctl repo, if the list does not
// exist locally, it is downloaded and cached for future use.
func GetRuntimeList(cacheDir string) (runtimes Runtimes, err error) {
	runtimesFile := filepath.Join(cacheDir, "runtimes.json")

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
		fmt.Fprintln(os.Stderr, "updating runtimes list...") // nolint:gas
		err = UpdateRuntimeList(cacheDir)
		if err != nil {
			return
		}
	}

	contents, err := ioutil.ReadFile(runtimesFile)
	if err != nil {
		err = errors.Wrap(err, "failed to read package cache file")
		return
	}

	err = json.Unmarshal(contents, &runtimes)
	return
}

// UpdateRuntimeList downloads a list of all runtime packages to a file in the cache directory
func UpdateRuntimeList(cacheDir string) (err error) {
	resp, err := http.Get("https://raw.githubusercontent.com/sampctl/runtimes/master/runtimes.json")
	if err != nil {
		return errors.Wrap(err, "failed to download package list")
	}

	if resp.StatusCode != 200 {
		return errors.Errorf("package list status %s", resp.Status)
	}

	var runtimes Runtimes
	err = json.NewDecoder(resp.Body).Decode(&runtimes)
	if err != nil {
		return errors.Wrap(err, "failed to decode package list")
	}

	contents, err := json.MarshalIndent(runtimes, "", "    ")
	if err != nil {
		return errors.Wrap(err, "failed to encode runtimes list")
	}

	runtimesFile := filepath.Join(cacheDir, "runtimes.json")
	err = ioutil.WriteFile(runtimesFile, contents, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to write package list to file")
	}

	return
}
