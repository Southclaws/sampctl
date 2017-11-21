package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Southclaws/sampctl/settings"
	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
)

// PrepareRuntime sets up a directory in ~/.samp that contains the server runtime
func PrepareRuntime(cacheDir, endpoint, version string) (err error) {
	dir := GetRuntimePath(cacheDir, version)

	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory")
	}

	errs := ValidateServerDir(dir, version)
	if errs != nil {
		fmt.Println(errs)
	}

	err = GetServerPackage(endpoint, version, dir)
	if err != nil {
		return errors.Wrap(err, "failed to get server package")
	}

	err = os.MkdirAll(filepath.Join(dir, "gamemodes"), 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create gamemodes directory")
	}

	err = os.MkdirAll(filepath.Join(dir, "filterscripts"), 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create filterscripts directory")
	}

	err = os.MkdirAll(filepath.Join(dir, "plugins"), 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create plugins directory")
	}

	return
}

// CopyFileToRuntime copies a specific file to execute to the specified version's runtime directory
func CopyFileToRuntime(cacheDir, version, filePath string) (err error) {
	fileName := filepath.Base(filePath)
	ext := filepath.Ext(filePath)
	justName := strings.TrimSuffix(fileName, ext)

	dir := GetRuntimePath(cacheDir, version)

	if ext == ".amx" {
		err := util.CopyFile(filePath, filepath.Join(dir, "gamemodes", fileName))
		if err != nil {
			return errors.Wrap(err, "failed to copy AMX to temporary runtime area")
		}
	} else {
		return errors.New("specified file is not an .amx")
	}

	config := settings.Config{
		Gamemodes:    []string{justName},
		RCONPassword: &[]string{"temp"}[0],
		Port:         &[]int{7777}[0],
	}
	err = config.GenerateJSON(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate temporary samp.json")
	}

	return
}

// GetRuntimePath returns the path from the cache directory where the runtime for a specific version
// of the server should exist.
func GetRuntimePath(cacheDir, version string) (runtimeDir string) {
	return filepath.Join(cacheDir, "runtime", version)
}
