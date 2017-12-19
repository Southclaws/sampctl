package rook

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func Run(pkg types.Package, cacheDir, endpoint, version, appVersion, build, platform string, container, forceBuild, forceEnsure bool) (err error) {
	runtimeDir := runtime.GetRuntimePath(cacheDir, version)

	err = runtime.PrepareRuntimeDirectory(cacheDir, endpoint, version, platform)
	if err != nil {
		return err
	}

	filename := util.FullPath(pkg.Output)
	if !util.Exists(filename) || forceBuild {
		filename, err = Build(&pkg, build, cacheDir, platform, forceEnsure)
		if err != nil {
			return err
		}
	}

	err = runtime.CopyFileToRuntime(cacheDir, version, filename)
	if err != nil {
		return err
	}

	config := types.MergeRuntimeDefault(&pkg.Runtime)
	config.Gamemodes = []string{strings.TrimSuffix(pkg.Output, ".amx")}
	config.WorkingDir = runtimeDir
	config.Version = &version
	config.Endpoint = &endpoint

	err = runtime.GenerateJSON(*config)
	if err != nil {
		return errors.Wrap(err, "failed to generate temporary samp.json")
	}

	fmt.Println("ensuring runtime installation", config.Plugins)
	err = runtime.Ensure(config)
	if err != nil {
		return errors.Wrap(err, "failed to ensure temporary runtime")
	}

	if container {
		err = runtime.RunContainer(*config, appVersion)
	} else {
		err = runtime.Run(*config)
	}
	return
}
