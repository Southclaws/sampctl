package rook

import (
	"context"
	"fmt"
	"io"
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
func (pcx *PackageContext) Run(ctx context.Context, output io.Writer, input io.Reader) (err error) {
	err = pcx.runPrepare(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to prepare package for running")
	}

	err = runtime.Run(ctx, pcx.ActualRuntime, pcx.CacheDir, true, false, output, input)
	if err != nil {
		return errors.Wrap(err, "failed to run package")
	}

	return
}

// RunWatch runs the Run code on file changes
func (pcx *PackageContext) RunWatch(ctx context.Context) (err error) {
	err = pcx.runPrepare(ctx)
	if err != nil {
		err = errors.Wrap(err, "failed to prepare")
		return
	}

	var (
		errorCh          = make(chan error)
		signals          = make(chan os.Signal, 1)
		trigger          = make(chan types.BuildProblems)
		running          atomic.Value
		ctxInner, cancel = context.WithCancel(ctx)
	)

	defer cancel()
	running.Store(false)

	go func() {
		errorCh <- pcx.BuildWatch(ctx, pcx.BuildName, pcx.ForceEnsure, pcx.BuildFile, pcx.Relative, trigger)
	}()
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	print.Verb(pcx.Package, "starting run watcher")

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
				fmt.Println("watch-run: killed existing runtime process")
				// re-create context and canceler
				ctxInner, cancel = context.WithCancel(ctx)
				defer cancel()
			}

			err = runtime.CopyFileToRuntime(pcx.CacheDir, pcx.ActualRuntime.Version, util.FullPath(pcx.Package.Output))
			if err != nil {
				err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
				print.Erro(err)
			}

			fmt.Println("watch-run: executing package code")
			go func() {
				running.Store(true)
				err = runtime.Run(ctxInner, pcx.ActualRuntime, pcx.CacheDir, true, false, os.Stdout, os.Stdin)
				running.Store(false)

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

func (pcx *PackageContext) runPrepare(ctx context.Context) (err error) {
	var (
		filename = filepath.Join(pcx.Package.LocalPath, pcx.Package.Output)
		problems types.BuildProblems
		canRun   = true
	)
	if !util.Exists(filename) || pcx.ForceBuild {
		problems, _, err = pcx.Build(ctx, pcx.BuildName, pcx.ForceEnsure, false, pcx.Relative, pcx.BuildFile)
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
		err = errors.New("build failed, can not run")
		return
	}

	print.Verb("getting runtime config")
	pcx.ActualRuntime, err = GetRuntimeConfig(pcx.Package, pcx.Runtime)
	if err != nil {
		return
	}

	pcx.ActualRuntime.Gamemodes = []string{strings.TrimSuffix(filepath.Base(pcx.Package.Output), ".amx")}

	pcx.ActualRuntime.AppVersion = pcx.AppVersion
	pcx.ActualRuntime.Format = pcx.Package.Format
	if pcx.Container {
		pcx.ActualRuntime.Container = &types.ContainerConfig{MountCache: true}
		pcx.ActualRuntime.Platform = "linux"
	} else {
		pcx.ActualRuntime.Platform = pcx.Platform
	}

	if !pcx.Package.Local {
		print.Verb(pcx.Package, "package is not local, preparing temporary runtime")

		scriptfiles := filepath.Join(pcx.Package.LocalPath, "scriptfiles")
		if !util.Exists(scriptfiles) {
			scriptfiles = ""
		}
		err = runtime.PrepareRuntimeDirectory(
			pcx.CacheDir,
			pcx.ActualRuntime.Version,
			pcx.ActualRuntime.Platform,
			scriptfiles)
		if err != nil {
			err = errors.Wrap(err, "failed to prepare temporary runtime area")
			return
		}

		err = runtime.CopyFileToRuntime(pcx.CacheDir, pcx.ActualRuntime.Version, filename)
		if err != nil {
			err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
			return
		}

		pcx.ActualRuntime.WorkingDir = runtime.GetRuntimePath(pcx.CacheDir, pcx.ActualRuntime.Version)
	} else {
		print.Verb(pcx.Package, "package is local, using working directory")

		pcx.ActualRuntime.WorkingDir = pcx.Package.LocalPath
		pcx.ActualRuntime.Format = pcx.Package.Format

		err = pcx.ActualRuntime.Validate()
		if err != nil {
			return
		}
	}

	print.Verb(pcx.Package, "gathering plugins pre-run")
	pcx.ActualRuntime.PluginDeps, err = pcx.GatherPlugins()
	if err != nil {
		err = errors.Wrap(err, "failed to gather plugins")
		return
	}

	print.Verb(pcx.Package, "ensuring runtime pre-run")
	err = runtime.Ensure(ctx, pcx.GitHub, &pcx.ActualRuntime, pcx.NoCache)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure runtime")
		return
	}

	return
}

// GetRuntimeConfig returns a matching runtime config by name from the package
// runtime list. If no name is specified, the first config is returned. If the
// package has no configurations, a default configuration is returned.
func GetRuntimeConfig(pkg types.Package, name string) (config types.Runtime, err error) {
	if len(pkg.Runtimes) > 0 {
		// if the user did not specify a specific runtime config, use the first
		// otherwise, search for a matching config by name
		if name == "" {
			config = *pkg.Runtimes[0]
			print.Verb(pkg, "searching", name, "in 'runtimes' list")
		} else {
			print.Verb(pkg, "using first config from 'runtimes' list")
			found := false
			for _, cfg := range pkg.Runtimes {
				if cfg.Name == name {
					config = *cfg
					found = true
					break
				}
			}
			if !found {
				err = errors.Errorf("no runtime config '%s'", name)
			}
		}
	} else if pkg.Runtime != nil {
		print.Verb(pkg, "using config from 'runtime' field")
		config = *pkg.Runtime
	} else {
		print.Verb(pkg, "using default config")
		config = types.Runtime{}
	}
	types.ApplyRuntimeDefaults(&config)

	return
}
