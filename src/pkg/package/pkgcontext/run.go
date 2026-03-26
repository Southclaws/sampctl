package pkgcontext

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

// Run will create a temporary server runtime and run the package output AMX as a gamemode using the
// runtime configuration in the package info.
func (pcx *PackageContext) Run(ctx context.Context, output io.Writer, input io.Reader) error {
	if err := pcx.RunPrepare(ctx); err != nil {
		return errors.Wrap(err, "failed to prepare package for running")
	}

	err := pcx.PackageServices.runtimeEnvironment().Run(
		ctx,
		pcx.ActualRuntime,
		pcx.runtimeRunOptions(output, input, true, false),
	)
	if err != nil {
		return errors.Wrap(err, "failed to run package")
	}

	return nil
}

// RunWatch runs the Run code on file changes.
func (pcx *PackageContext) RunWatch(ctx context.Context) (err error) {
	if err = pcx.RunPrepare(ctx); err != nil {
		return errors.Wrap(err, "failed to prepare")
	}

	var (
		errorCh              = make(chan error, 1)
		signals, stopSignals = newTerminationSignals()
		trigger              = make(chan build.Problems)
		runtime              watchedRuntime
	)
	defer stopSignals()
	defer runtime.Stop()

	go func() {
		errorCh <- pcx.BuildWatch(ctx, BuildOptions{
			Name:      pcx.BuildName,
			Ensure:    pcx.ForceEnsure,
			BuildFile: pcx.BuildFile,
			Relative:  pcx.Relative,
			Trigger:   trigger,
		})
	}()

	print.Verb(pcx.Package, "starting run watcher")

loop:
	for {
		select {
		case sig := <-signals:
			fmt.Println("")
			print.Info("signal received", sig, "stopping run watcher...")
			break loop

		case err = <-errorCh:
			break loop

		case problems := <-trigger:
			print.Info("build finished")
			if hasBlockingBuildProblem(problems) {
				continue
			}

			runtime.Stop()

			outputPath, pathErr := pcx.runtimeOutputPath()
			if pathErr != nil {
				err = pathErr
				print.Erro(err)
				continue
			}
			if err = pcx.copyRuntimeBinary(outputPath); err != nil {
				print.Erro(err)
				continue
			}

			print.Verb("watch-run: executing package code")
			runtime.Restart(ctx, pcx.startWatchedRuntime)
		}
	}

	print.Info("finished running run watcher")

	return err
}

// RunPrepare prepares the context directory for executing the server.
func (pcx *PackageContext) RunPrepare(ctx context.Context) (err error) {
	var (
		filename = packagePath(pcx.Package.LocalPath, pcx.Package.Output)
		problems build.Problems
		canRun   = true
	)
	filename, err = fs.Abs(filename)
	if err != nil {
		return errors.Wrap(err, "failed to resolve package output path")
	}
	if !fs.Exists(filename) || pcx.ForceBuild {
		problems, _, err = pcx.Build(ctx, BuildOptions{
			Name:      pcx.BuildName,
			Ensure:    pcx.ForceEnsure,
			Relative:  pcx.Relative,
			BuildFile: pcx.BuildFile,
		})
		if err != nil {
			return
		}

		for _, problem := range problems {
			if problem.Severity > build.ProblemWarning {
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
	pcx.ActualRuntime, err = pcx.Package.GetRuntimeConfig(pcx.Runtime)
	if err != nil {
		return
	}

	pcx.ActualRuntime.Gamemodes = []string{strings.TrimSuffix(filepath.Base(pcx.Package.Output), ".amx")}
	pcx.ActualRuntime.AppVersion = pcx.AppVersion
	pcx.ActualRuntime.Format = pcx.Package.Format
	if pcx.Container {
		pcx.ActualRuntime.Container = &run.ContainerConfig{MountCache: true}
		pcx.ActualRuntime.Platform = "linux"
	} else {
		pcx.ActualRuntime.Platform = pcx.Platform
	}

	if !pcx.Package.EffectiveLocal() {
		if err = pcx.prepareTemporaryRuntime(filename); err != nil {
			return err
		}
	} else {
		print.Verb(pcx.Package, "package is local, using working directory")
		pcx.ActualRuntime.WorkingDir = pcx.Package.LocalPath
		pcx.ActualRuntime.Format = pcx.Package.Format
		if err = pcx.ActualRuntime.Validate(); err != nil {
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
	err = pcx.PackageServices.runtimeEnvironment().Ensure(ctx, pcx.GitHub, &pcx.ActualRuntime, pcx.NoCache)
	if err != nil {
		err = errors.Wrap(err, "failed to ensure runtime")
		return
	}

	print.Verb("generating server configuration file")
	err = pcx.PackageServices.runtimeEnvironment().GenerateConfig(&pcx.ActualRuntime)
	if err != nil {
		return errors.Wrap(err, "failed to generate server configuration")
	}

	return nil
}

func (pcx *PackageContext) runtimeRunOptions(output io.Writer, input io.Reader, passArgs, recover bool) runtimepkg.RunOptions {
	return runtimepkg.RunOptions{
		CacheDir: pcx.CacheDir,
		PassArgs: passArgs,
		Recover:  recover,
		Output:   output,
		Input:    input,
	}
}

func (pcx *PackageContext) runtimeOutputPath() (string, error) {
	outputPath, err := fs.Abs(packagePath(pcx.Package.LocalPath, pcx.Package.Output))
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve package output path")
	}
	return outputPath, nil
}

func (pcx *PackageContext) copyRuntimeBinary(outputPath string) error {
	err := pcx.PackageServices.runtimeEnvironment().CopyFileToRuntime(pcx.CacheDir, pcx.ActualRuntime.Version, outputPath)
	if err != nil {
		return errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
	}
	return nil
}

func (pcx *PackageContext) startWatchedRuntime(ctx context.Context, running *atomic.Bool) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		running.Store(true)
		defer running.Store(false)

		err := pcx.PackageServices.runtimeEnvironment().Run(
			ctx,
			pcx.ActualRuntime,
			pcx.runtimeRunOptions(os.Stdout, os.Stdin, true, false),
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			print.Erro(err)
		}

		print.Verb("watch-run: finished")
		done <- err
	}()
	return done
}

func hasBlockingBuildProblem(problems build.Problems) bool {
	for _, problem := range problems {
		if problem.Severity > build.ProblemWarning {
			return true
		}
	}
	return false
}

func (pcx *PackageContext) prepareTemporaryRuntime(filename string) error {
	print.Verb(pcx.Package, "package is not local, preparing temporary runtime")

	scriptfiles := filepath.Join(pcx.Package.LocalPath, "scriptfiles")
	if !fs.Exists(scriptfiles) {
		scriptfiles = ""
	}
	err := pcx.PackageServices.runtimeEnvironment().PrepareRuntimeDirectory(
		pcx.CacheDir,
		pcx.ActualRuntime.Version,
		pcx.ActualRuntime.Platform,
		scriptfiles,
	)
	if err != nil {
		return errors.Wrap(err, "failed to prepare temporary runtime area")
	}

	if err = pcx.PackageServices.runtimeEnvironment().CopyFileToRuntime(pcx.CacheDir, pcx.ActualRuntime.Version, filename); err != nil {
		return errors.Wrap(err, "failed to copy amx file to temporary runtime directory")
	}

	pcx.ActualRuntime.WorkingDir = runtimepkg.GetRuntimePath(pcx.CacheDir, pcx.ActualRuntime.Version)
	return nil
}
