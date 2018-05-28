package rook

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	rt "runtime"
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

// Runner stores state and configuration for running a server instance
type Runner struct {
	Pkg         types.Package        // Package that this runner targets
	Runtime     string               // the runtime config to use, defaults to `default`
	Container   bool                 // whether or not to run the package in a container
	AppVersion  string               // the version of sampctl
	GitHub      *github.Client       // GitHub client for downloading plugins
	Auth        transport.AuthMethod // Authentication method for git
	CacheDir    string               // Cache directory
	Build       string               // Build configuration to use from pkg.Builds
	ForceBuild  bool                 // Force a build before running
	ForceEnsure bool                 // Force an ensure before building before running
	NoCache     bool                 // Don't use a cache, download all plugin dependencies
	BuildFile   string               // File to increment build number
	Relative    bool                 // Show output as relative paths
}

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func (runner *Runner) Run(ctx context.Context, output io.Writer, input io.Reader) (err error) {
	err = runner.prepare(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to prepare package for running")
	}

	err = runtime.Run(ctx, *runner.Pkg.Runtime, runner.CacheDir, true, false, output, input)
	if err != nil {
		return errors.Wrap(err, "failed to run package")
	}

	return
}

// RunWatch runs the Run code on file changes
func (runner *Runner) RunWatch(ctx context.Context) (err error) {
	err = runner.prepare(ctx)
	if err != nil {
		err = errors.Wrap(err, "failed to prepare")
		return
	}

	if runner.Pkg.Runtime.Mode == types.Server {
		err = errors.New("cannot use --watch with runtime mode 'server'")
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
		errorCh <- BuildWatch(
			ctx,
			runner.GitHub,
			runner.Auth,
			&runner.Pkg,
			runner.Build,
			runner.CacheDir,
			runner.Pkg.Runtime.Platform,
			runner.ForceEnsure,
			runner.BuildFile,
			runner.Relative,
			trigger)
	}()
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	print.Verb(runner.Pkg, "starting run watcher")

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

			err = runtime.CopyFileToRuntime(runner.CacheDir, runner.Pkg.Runtime.Version, util.FullPath(runner.Pkg.Output))
			if err != nil {
				err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
				print.Erro(err)
			}

			fmt.Println("watch-run: executing package code")
			go func() {
				running.Store(true)
				err = runtime.Run(ctxInner, *runner.Pkg.Runtime, runner.CacheDir, true, false, os.Stdout, os.Stdin)
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

func (runner *Runner) prepare(ctx context.Context) (err error) {
	var (
		filename = filepath.Join(runner.Pkg.LocalPath, runner.Pkg.Output)
		problems types.BuildProblems
		canRun   = true
	)
	if !util.Exists(filename) || runner.ForceBuild {
		problems, _, err = Build(
			ctx,
			runner.GitHub,
			runner.Auth,
			&runner.Pkg,
			runner.Build,
			runner.CacheDir,
			runner.Pkg.Runtime.Platform,
			runner.ForceEnsure,
			false,
			runner.Relative,
			runner.BuildFile)
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

	runner.Pkg.Runtime = GetRuntimeConfig(runner.Pkg, runner.Runtime)
	runner.Pkg.Runtime.Gamemodes = []string{strings.TrimSuffix(filepath.Base(runner.Pkg.Output), ".amx")}

	if runner.Container {
		runner.Pkg.Runtime.AppVersion = runner.AppVersion
		runner.Pkg.Runtime.Container = &types.ContainerConfig{MountCache: true}
		runner.Pkg.Runtime.Platform = "linux"
	} else {
		runner.Pkg.Runtime.Platform = rt.GOOS
	}

	if !runner.Pkg.Local {
		scriptfiles := filepath.Join(runner.Pkg.LocalPath, "scriptfiles")
		if !util.Exists(scriptfiles) {
			scriptfiles = ""
		}
		err = runtime.PrepareRuntimeDirectory(
			runner.CacheDir,
			runner.Pkg.Runtime.Version,
			runner.Pkg.Runtime.Platform,
			scriptfiles)
		if err != nil {
			err = errors.Wrap(err, "failed to prepare temporary runtime area")
			return
		}

		err = runtime.CopyFileToRuntime(runner.CacheDir, runner.Pkg.Runtime.Version, filename)
		if err != nil {
			err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
			return
		}

		runner.Pkg.Runtime.WorkingDir = runtime.GetRuntimePath(runner.CacheDir, runner.Pkg.Runtime.Version)
	}

	runner.Pkg.Runtime.PluginDeps = []versioning.DependencyMeta{}
	for _, pluginMeta := range runner.Pkg.AllPlugins {
		print.Verb("read plugin from dependency:", pluginMeta)
		runner.Pkg.Runtime.PluginDeps = append(runner.Pkg.Runtime.PluginDeps, pluginMeta)
	}
	print.Verb(runner.Pkg.Runtime.PluginDeps)

	err = runtime.Ensure(ctx, runner.GitHub, runner.Pkg.Runtime, runner.NoCache)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure temporary runtime")
		return
	}

	return
}

// GetRuntimeConfig returns a matching runtime config by name from the package
// runtime list. If no name is specified, the first config is returned. If the
// package has no configurations, a default configuration is returned.
func GetRuntimeConfig(pkg types.Package, name string) (config *types.Runtime) {
	def := types.GetRuntimeDefault()

	// if there are no runtimes at all, use default
	if len(pkg.Runtimes) == 0 && pkg.Runtime == nil {
		return def
	}

	// if the user did not specify a specific runtime config, use the first
	// otherwise, search for a matching config by name
	if name == "default" {
		if pkg.Runtime != nil {
			config = pkg.Runtime
		} else {
			config = pkg.Runtimes[0]
		}
	} else {
		for _, cfg := range pkg.Runtimes {
			if cfg.Name == name {
				config = cfg
				break
			}
		}
	}

	if config == nil {
		print.Warn("No runtime config called:", name, "using default")
		return def
	}

	if config.RCONPassword == nil {
		config.RCONPassword = def.RCONPassword
	}
	if config.Port == nil {
		config.Port = def.Port
	}

	return
}
