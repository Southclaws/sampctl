package rook

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func (pkg Package) Run(cacheDir, endpoint, version, appVersion, build string, container, forceBuild, forceEnsure bool) (err error) {
	runtimeDir := runtime.GetRuntimePath(cacheDir, version)

	err = runtime.PrepareRuntimeDirectory(cacheDir, endpoint, version)
	if err != nil {
		return err
	}

	filename := util.FullPath(pkg.Output)
	if !util.Exists(filename) || forceBuild {
		filename, err = pkg.Build(build, forceEnsure)
		if err != nil {
			return err
		}
	}

	err = runtime.CopyFileToRuntime(cacheDir, version, filename)
	if err != nil {
		return err
	}

	config := runtime.MergeDefaultConfig(pkg.Runtime)
	config.Gamemodes = []string{strings.TrimSuffix(pkg.Output, ".amx")}

	err = config.GenerateJSON(runtimeDir)
	if err != nil {
		return errors.Wrap(err, "failed to generate temporary samp.json")
	}

	if container {
		err = runtime.RunContainer(endpoint, version, runtimeDir, appVersion)
	} else {
		err = runtime.Run(endpoint, version, runtimeDir)
	}
	return
}
