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

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func Run(ctx context.Context, gh *github.Client, auth transport.AuthMethod, pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool, buildFile string) (err error) {
	config, err := runPrepare(ctx, gh, auth, pkg, cfg, cacheDir, build, forceBuild, forceEnsure, noCache, buildFile)
	if err != nil {
		return
	}

	err = runtime.Run(ctx, *config, cacheDir, os.Stdout)

	return
}

// RunWatch runs the Run code on file changes
func RunWatch(ctx1 context.Context, gh *github.Client, auth transport.AuthMethod, pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool, buildFile string) (err error) {
	config, err := runPrepare(ctx1, gh, auth, pkg, cfg, cacheDir, build, forceBuild, forceEnsure, noCache, buildFile)
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
		trigger     = make(chan types.BuildProblems)
		running     atomic.Value
		ctx, cancel = context.WithCancel(ctx1)
	)

	defer cancel()
	running.Store(false)

	go func() {
		errorCh <- BuildWatch(ctx, gh, auth, &pkg, build, cacheDir, cfg.Platform, forceEnsure, buildFile, trigger)
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
				defer cancel()
			}

			err = runtime.CopyFileToRuntime(cacheDir, cfg.Version, util.FullPath(pkg.Output))
			if err != nil {
				err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
				print.Erro(err)
			}

			fmt.Println("watch-run: executing package code")
			go func() {
				err = runtime.Run(ctx, *config, cacheDir, os.Stdout)
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

func runPrepare(ctx context.Context, gh *github.Client, auth transport.AuthMethod, pkg types.Package, cfg types.Runtime, cacheDir, build string, forceBuild, forceEnsure, noCache bool, buildFile string) (config *types.Runtime, err error) {
	var (
		filename = filepath.Join(pkg.Local, pkg.Output)
		problems types.BuildProblems
		canRun   = true
	)
	if !util.Exists(filename) || forceBuild {
		problems, _, err = Build(ctx, gh, auth, &pkg, build, cacheDir, cfg.Platform, forceEnsure, false, buildFile)
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

	err = runtime.PrepareRuntimeDirectory(cacheDir, cfg.Endpoint, cfg.Version, cfg.Platform)
	if err != nil {
		err = errors.Wrap(err, "failed to prepare temporary runtime area")
		return
	}

	err = runtime.CopyFileToRuntime(cacheDir, cfg.Version, filename)
	if err != nil {
		err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
		return
	}

	config = types.MergeRuntimeDefault(pkg.Runtime)

	config.Platform = cfg.Platform
	config.AppVersion = cfg.AppVersion
	config.Version = cfg.Version
	config.Endpoint = cfg.Endpoint
	config.Container = cfg.Container

	config.Gamemodes = []string{strings.TrimSuffix(filepath.Base(pkg.Output), ".amx")}
	config.WorkingDir = runtime.GetRuntimePath(cacheDir, cfg.Version)

	config.PluginDeps = []versioning.DependencyMeta{}
	for _, pluginMeta := range pkg.AllPlugins {
		print.Verb("read plugin from dependency:", pluginMeta)
		config.PluginDeps = append(config.PluginDeps, pluginMeta)
	}
	print.Verb(config.PluginDeps)

	err = config.ToJSON()
	if err != nil {
		err = errors.Wrap(err, "failed to generate temporary samp.json")
		return
	}

	err = runtime.Ensure(ctx, gh, config, noCache, true)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure temporary runtime")
		return
	}

	return
}
