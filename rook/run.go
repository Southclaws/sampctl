package rook

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func Run(pkg types.Package, cacheDir, endpoint, version, appVersion, build, platform string, container, forceBuild, forceEnsure, noCache bool) (err error) {
	runtimeDir := runtime.GetRuntimePath(cacheDir, version)

	err = runtime.PrepareRuntimeDirectory(cacheDir, endpoint, version, platform)
	if err != nil {
		return err
	}

	var (
		filename = util.FullPath(pkg.Output)
		problems []types.BuildProblem
		canRun   = true
	)
	if !util.Exists(filename) || forceBuild {
		problems, _, err = Build(&pkg, build, cacheDir, platform, forceEnsure)
		if err != nil {
			return err
		}

		for _, problem := range problems {
			if problem.Severity > types.ProblemWarning {
				canRun = false
			}
			fmt.Println(problem)
		}
	}
	if !canRun {
		color.Red("Build failed, can not run")
	}

	err = runtime.CopyFileToRuntime(cacheDir, version, filename)
	if err != nil {
		return err
	}

	config := types.MergeRuntimeDefault(pkg.Runtime)
	config.Platform = platform
	config.Gamemodes = []string{strings.TrimSuffix(filepath.Base(pkg.Output), ".amx")}
	config.WorkingDir = runtimeDir
	config.Version = version
	config.Endpoint = endpoint

	config.Plugins = []types.Plugin{}
	for _, pluginMeta := range pkg.AllPlugins {
		config.Plugins = append(config.Plugins, types.Plugin(pluginMeta.String()))
	}

	err = runtime.GenerateJSON(*config)
	if err != nil {
		return errors.Wrap(err, "failed to generate temporary samp.json")
	}

	err = runtime.Ensure(config, noCache)
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
