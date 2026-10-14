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
		pcx.runtimeRunOptions(output, input, false),
	)
	if err != nil {
		return errors.Wrap(err, "failed to run package")
	}

	return nil
}

// RunWatch runs the Run code on file changes.
func (pcx *PackageContext) RunWatch(ctx context.Context) (err error) {
	if err = pcx.RunPrepare(ctx); err != nil {
		return errors.Wrap(err, "failed to prepare package for run watcher")
	}

	watchCtx, cancel := context.WithCancel(ctx)

	var (
		errorCh              = make(chan error, 1)
		signals, stopSignals = newTerminationSignals()
		trigger              = make(chan build.Problems)
		runtime              watchedRuntime
		buildDone            = make(chan struct{})
	)
	defer stopSignals()
	defer func() {
		cancel()
		runtime.Stop()
		<-buildDone
	}()

	go func() {
		defer close(buildDone)

		errorCh <- pcx.BuildWatch(watchCtx, BuildOptions{
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
		case <-ctx.Done():
			err = ctx.Err()
			break loop

		case sig := <-signals:
			fmt.Println("")
			print.Info("Stopping run watcher after signal", sig)
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
			if err = pcx.stageRuntimeOutput(outputPath); err != nil {
				print.Erro(err)
				continue
			}

			print.Verb("watch-run: executing package code")
			runtime.Restart(watchCtx, pcx.startWatchedRuntime)
		}
	}

	print.Info("Run watcher stopped")

	return err
}

// RunPrepare prepares the context directory for executing the server.
func (pcx *PackageContext) RunPrepare(ctx context.Context) (err error) {
	var (
		problems build.Problems
		canRun   = true
	)
	filename, err := pcx.runtimeOutputPath()
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
		err = errors.New("cannot run package because the build failed")
		return
	}

	print.Verb("getting runtime config")
	pcx.ActualRuntime, err = pcx.Package.GetRuntimeConfig(pcx.Runtime)
	if err != nil {
		return
	}

	pcx.ActualRuntime.Gamemodes = []string{strings.TrimSuffix(filepath.Base(filename), ".amx")}
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
		pcx.ActualRuntime.WorkingDir = pcx.Package.RuntimeWorkingDir()
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

	if pcx.Package.EffectiveLocal() && pcx.Package.RuntimeDir != "" {
		if err = pcx.copyOutputToLocalRuntime(filename); err != nil {
			return err
		}
	}

	print.Verb("generating server configuration file")
	err = pcx.PackageServices.runtimeEnvironment().GenerateConfig(&pcx.ActualRuntime)
	if err != nil {
		return errors.Wrap(err, "failed to generate server configuration")
	}

	return nil
}

func (pcx *PackageContext) runtimeRunOptions(output io.Writer, input io.Reader, recover bool) runtimepkg.RunOptions {
	return runtimepkg.RunOptions{
		CacheDir:   pcx.CacheDir,
		ProjectDir: pcx.Package.LocalPath,
		Args:       pcx.containerRunArgs(),
		Recover:    recover,
		Output:     output,
		Input:      input,
	}
}

func (pcx *PackageContext) containerRunArgs() []string {
	args := []string{"--dir", "/samp"}
	if pcx.BuildName != "" {
		args = append(args, "--build", pcx.BuildName)
	}
	for _, flag := range []struct {
		name    string
		enabled bool
	}{
		{"--forceBuild", pcx.ForceBuild},
		{"--forceEnsure", pcx.ForceEnsure},
		{"--noCache", pcx.NoCache},
		{"--relativePaths", pcx.Relative},
	} {
		if flag.enabled {
			args = append(args, flag.name)
		}
	}
	if pcx.BuildFile != "" {
		buildFile := pcx.BuildFile
		if filepath.IsAbs(buildFile) {
			if relative, err := filepath.Rel(pcx.Package.LocalPath, buildFile); err == nil && filepath.IsLocal(relative) {
				buildFile = filepath.ToSlash(filepath.Join("/samp", relative))
			}
		}
		args = append(args, "--buildFile", buildFile)
	}
	if pcx.Runtime != "" {
		args = append(args, pcx.Runtime)
	}

	return args
}

func (pcx *PackageContext) runtimeOutputPath() (string, error) {
	config := pcx.Package.GetBuildConfig(pcx.BuildName)
	output := pcx.Package.Output
	if config != nil && config.Output != "" {
		output = config.Output
	}

	outputPath, err := fs.Abs(packagePath(pcx.Package.LocalPath, output))
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve package output path")
	}

	return outputPath, nil
}

func (pcx *PackageContext) stageRuntimeOutput(outputPath string) error {
	if pcx.Package.EffectiveLocal() && pcx.Package.RuntimeDir != "" {
		return pcx.copyOutputToLocalRuntime(outputPath)
	}

	return pcx.copyRuntimeBinary(outputPath)
}

func (pcx *PackageContext) copyOutputToLocalRuntime(outputPath string) error {
	targetPath := filepath.Join(pcx.ActualRuntime.WorkingDir, "gamemodes", filepath.Base(outputPath))

	sourceInfo, err := os.Stat(outputPath)
	if err != nil {
		return errors.Wrap(err, "failed to stat package output")
	}

	targetInfo, targetErr := os.Stat(targetPath)
	if targetErr == nil && os.SameFile(sourceInfo, targetInfo) {
		return nil
	}
	if targetErr != nil && !os.IsNotExist(targetErr) {
		return errors.Wrap(targetErr, "failed to stat runtime output")
	}

	source, err := os.Open(outputPath)
	if err != nil {
		return errors.Wrap(err, "failed to open package output")
	}

	copyErr := fs.WriteFromReaderAtomic(targetPath, source, fs.PermDirShared, fs.PermFileShared)
	closeErr := source.Close()
	if copyErr != nil {
		return errors.Wrap(copyErr, "failed to copy package output to local runtime")
	}
	if closeErr != nil {
		return errors.Wrap(closeErr, "failed to close package output")
	}

	return nil
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
			pcx.runtimeRunOptions(os.Stdout, os.Stdin, false),
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
