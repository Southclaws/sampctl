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
func Run(pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool) (err error) {
	runtimeDir := runtime.GetRuntimePath(cacheDir, cfg.Version)

	err = runtime.PrepareRuntimeDirectory(cacheDir, cfg.Endpoint, cfg.Version, cfg.Platform)
	if err != nil {
		return err
	}

	var (
		filename = util.FullPath(pkg.Output)
		problems []types.BuildProblem
		canRun   = true
	)
	if !util.Exists(filename) || forceBuild {
		problems, _, err = Build(&pkg, build, cacheDir, cfg.Platform, forceEnsure)
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

	err = runtime.CopyFileToRuntime(cacheDir, cfg.Version, filename)
	if err != nil {
		return err
	}

	config := types.MergeRuntimeDefault(pkg.Runtime)

	config.Platform = cfg.Platform
	config.AppVersion = cfg.AppVersion
	config.Version = cfg.Version
	config.Endpoint = cfg.Endpoint
	config.Container = cfg.Container

	config.Gamemodes = []string{strings.TrimSuffix(filepath.Base(pkg.Output), ".amx")}
	config.WorkingDir = runtimeDir

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

	err = runtime.Run(*config, cacheDir)

	return
}
