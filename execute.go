package main

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
)

func prepareTemporaryServer(endpoint, version, dir, filePath string) (err error) {
	errs := ValidateServerDir(dir, version)
	if errs != nil {
		fmt.Println(errs)
		err = GetPackage(endpoint, version, dir)
		if err != nil {
			return errors.Wrap(err, "failed to get server package")
		}
	}
	ext := filepath.Ext(filePath)
	if ext == ".pwn" {
		// compile
		// set filePath to output amx location
	} else if ext == ".amx" {
		err := CopyFile(filePath, filepath.Join(dir, "gamemodes", filePath))
		if err != nil {
			return errors.Wrap(err, "failed to copy AMX to temporary runtime area")
		}
	}

	// generate Config
	// write to samp.json

	return
}
