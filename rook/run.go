package rook

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func Run(pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool, buildFile string) (err error) {
	config, err := runPrepare(pkg, cfg, cacheDir, build, forceBuild, forceEnsure, noCache, buildFile)
	if err != nil {
		return
	}

	err = runtime.Run(context.Background(), *config, cacheDir)

	return
}

// RunWatch runs the Run code on file changes
func RunWatch(pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool, buildFile string) (err error) {
	config, err := runPrepare(pkg, cfg, cacheDir, build, forceBuild, forceEnsure, noCache, buildFile)
	if err != nil {
		err = errors.Wrap(err, "failed to prepare")
		return
	}

	if config.Mode == types.Server {
		err = errors.New("cannot use --watch with runtime mode 'server'")
		return
	}

	var (
		errorCh     = make(chan error)
		signals     = make(chan os.Signal, 1)
		trigger     = make(chan []types.BuildProblem)
		running     atomic.Value
		ctx, cancel = context.WithCancel(context.Background())
	)

	running.Store(false)

	go func() {
		errorCh <- BuildWatch(ctx, &pkg, build, cacheDir, cfg.Platform, forceEnsure, buildFile, trigger)
	}()
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	print.Verb(pkg, "starting run watcher")

loop:
	for {
		select {
		case sig := <-signals:
			fmt.Println("") // insert newline after the ^C
			print.Info("signal received", sig, "stopping run watcher...")
			break loop

		case err = <-errorCh:
			cancel()
			break loop

		case problems := <-trigger:
			print.Info("build finished")
			for _, problem := range problems {
				if problem.Severity > types.ProblemWarning {
					continue loop
				}
			}

			if running.Load().(bool) {
				fmt.Println("watch-run: killing existing runtime process")
				cancel()
				fmt.Println("watch-run: finished")
				// re-create context and canceler
				ctx, cancel = context.WithCancel(context.Background())
			}

			err = runtime.CopyFileToRuntime(cacheDir, cfg.Version, util.FullPath(pkg.Output))
			if err != nil {
				err = errors.Wrap(err, "failed to copy amx file to temprary runtime directory")
				print.Erro(err)
			}

			fmt.Println("watch-run: executing package code")
			go func() {
				err = runtime.Run(ctx, *config, cacheDir)
				if err != nil {
					print.Erro(err)
				}
				fmt.Println("watch-run: finished")
			}()
		}
	}

	print.Info("finished running run watcher")

	return
}

func runPrepare(pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool, buildFile string) (config *types.Runtime, err error) {
	var (
		filename = util.FullPath(pkg.Output)
		problems []types.BuildProblem
		canRun   = true
	)
	if !util.Exists(filename) || forceBuild {
		problems, _, err = Build(&pkg, build, cacheDir, cfg.Platform, forceEnsure, false, buildFile)
		if err != nil {
			return
		}

		for _, problem := range problems {
			if problem.Severity > types.ProblemWarning {
				canRun = false
				break
			}
		}
	}
	if !canRun {
		err = errors.New("Build failed, can not run")
		return
	}

	err = runtime.CopyFileToRuntime(cacheDir, cfg.Version, filename)
	if err != nil {
		err = errors.Wrap(err, "failed to copy amx file to temprary runtime directory")
		return
	}

	runtimeDir := runtime.GetRuntimePath(cacheDir, cfg.Version)

	err = runtime.PrepareRuntimeDirectory(cacheDir, cfg.Endpoint, cfg.Version, cfg.Platform)
	if err != nil {
		return
	}

	config = types.MergeRuntimeDefault(pkg.Runtime)

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
		err = errors.Wrap(err, "failed to generate temporary samp.json")
		return
	}

	err = runtime.Ensure(config, noCache, true)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure temporary runtime")
		return
	}

	return
}
