package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// PrepareRuntime sets up a directory in ~/.samp that contains the server runtime and a compiler
func PrepareRuntime(endpoint, serverVersion, dir string) (err error) {
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory")
	}
	errs := ValidateServerDir(dir, version)
	if errs != nil {
		fmt.Println(errs)
		err = GetServerPackage(endpoint, version, dir)
		if err != nil {
			return errors.Wrap(err, "failed to get server package")
		}
	}

	return
}

// CopyFileToRuntime copies a specific file to execute to the specified version's runtime directory
func CopyFileToRuntime(cacheDir, version, filePath string) (err error) {
	fileName := filepath.Base(filePath)
	ext := filepath.Ext(filePath)
	justName := strings.TrimSuffix(fileName, ext)

	dir := filepath.Join(cacheDir, "runtime", version)

	if ext == ".pwn" {
		// compile
		// set filePath to output amx location
	} else if ext == ".amx" {
		err := CopyFile(filePath, filepath.Join(dir, "gamemodes", fileName))
		if err != nil {
			return errors.Wrap(err, "failed to copy AMX to temporary runtime area")
		}
	}

	config := Config{
		Gamemodes:    []string{justName},
		RCONPassword: &[]string{"temp"}[0],
	}
	err = config.GenerateJSON(dir)
	if err != nil {
		return errors.Wrap(err, "failed to generate temporary samp.json")
	}

	return
}
