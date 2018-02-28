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
	Config      types.Runtime        // Runtime configuration
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
func (runner Runner) Run(ctx context.Context, output io.Writer, input io.Reader) (err error) {
	config, err := runner.prepare(ctx)
	if err != nil {
		return
	}
	runner.Config = *config

	err = runtime.Run(ctx, runner.Config, runner.CacheDir, true, output, input)

	return
}

// RunWatch runs the Run code on file changes
func (runner Runner) RunWatch(ctx1 context.Context) (err error) {
	config, err := runner.prepare(ctx1)
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
		errorCh <- BuildWatch(
			ctx,
			runner.GitHub,
			runner.Auth,
			&runner.Pkg,
			runner.Build,
			runner.CacheDir,
			runner.Config.Platform,
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
				fmt.Println("watch-run: finished")
				// re-create context and canceler
				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()
			}

			err = runtime.CopyFileToRuntime(runner.CacheDir, runner.Config.Version, util.FullPath(runner.Pkg.Output))
			if err != nil {
				err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
				print.Erro(err)
			}

			fmt.Println("watch-run: executing package code")
			go func() {
				err = runtime.Run(ctx, runner.Config, runner.CacheDir, true, os.Stdout, os.Stdin)
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

func (runner Runner) prepare(ctx context.Context) (config *types.Runtime, err error) {
	var (
		filename = filepath.Join(runner.Pkg.Local, runner.Pkg.Output)
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
			runner.Config.Platform,
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

	err = runtime.PrepareRuntimeDirectory(runner.CacheDir, runner.Config.Endpoint, runner.Config.Version, runner.Config.Platform)
	if err != nil {
		err = errors.Wrap(err, "failed to prepare temporary runtime area")
		return
	}

	err = runtime.CopyFileToRuntime(runner.CacheDir, runner.Config.Version, filename)
	if err != nil {
		err = errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
		return
	}

	config = types.MergeRuntimeDefault(runner.Pkg.Runtime)

	config.Platform = runner.Config.Platform
	config.AppVersion = runner.Config.AppVersion
	config.Version = runner.Config.Version
	config.Endpoint = runner.Config.Endpoint
	config.Container = runner.Config.Container

	config.Gamemodes = []string{strings.TrimSuffix(filepath.Base(runner.Pkg.Output), ".amx")}
	config.WorkingDir = runtime.GetRuntimePath(runner.CacheDir, runner.Config.Version)

	config.PluginDeps = []versioning.DependencyMeta{}
	for _, pluginMeta := range runner.Pkg.AllPlugins {
		print.Verb("read plugin from dependency:", pluginMeta)
		config.PluginDeps = append(config.PluginDeps, pluginMeta)
	}
	print.Verb(config.PluginDeps)

	err = config.ToJSON()
	if err != nil {
		err = errors.Wrap(err, "failed to generate temporary samp.json")
		return
	}

	err = runtime.Ensure(ctx, runner.GitHub, &runner.Config, runner.NoCache)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure temporary runtime")
		return
	}

	return
}
