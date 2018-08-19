package runtime

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
)

// PrepareRuntimeDirectory sets up a directory in ~/.samp that contains the server runtime
func PrepareRuntimeDirectory(cacheDir, version, platform, scriptfiles string) (err error) {
	dir := GetRuntimePath(cacheDir, version)

	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory")
	}

	err = GetServerPackage(version, dir, platform)
	if err != nil {
		return errors.Wrap(err, "failed to get server package")
	}

	err = os.MkdirAll(filepath.Join(dir, "gamemodes"), 0700)
	if err != nil {
		return errors.Wrap(err, "failed to create gamemodes directory")
	}

	err = os.MkdirAll(filepath.Join(dir, "filterscripts"), 0700)
	if err != nil {
		return errors.Wrap(err, "failed to create filterscripts directory")
	}

	err = os.MkdirAll(filepath.Join(dir, "plugins"), 0700)
	if err != nil {
		return errors.Wrap(err, "failed to create plugins directory")
	}

	scriptfilesTmp := filepath.Join(dir, "scriptfiles")
	err = os.RemoveAll(scriptfilesTmp)
	if err != nil {
		return errors.Wrap(err, "failed to remove scriptfiles directory")
	}

	if scriptfiles != "" {
		err = os.Symlink(scriptfiles, scriptfilesTmp)
		if err != nil {
			print.Erro("Failed to create scriptfiles symlink:", err)
		}
	}

	return nil
}

// CopyFileToRuntime copies a specific file to execute to the specified version's runtime directory
func CopyFileToRuntime(cacheDir, version, filePath string) (err error) {
	fileName := filepath.Base(filePath)
	ext := filepath.Ext(filePath)
	dir := GetRuntimePath(cacheDir, version)

	if ext != ".amx" {
		return errors.New("specified file is not an .amx")
	}

	err = util.CopyFile(filePath, filepath.Join(dir, "gamemodes", fileName))
	if err != nil {
		return errors.Wrap(err, "failed to copy AMX to temporary runtime area")
	}

	return
}

// GetRuntimePath returns the path from the cache directory where the runtime for a specific version
// of the server should exist.
func GetRuntimePath(cacheDir, version string) (runtimeDir string) {
	return filepath.Join(cacheDir, "runtime", version)
}
