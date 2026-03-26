package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

const buildWatchDebounce = 750 * time.Millisecond

type buildWatchResult struct {
	problems    build.Problems
	err         error
	eventName   string
	buildNumber uint32
}

type BuildOptions struct {
	Name      string
	Ensure    bool
	DryRun    bool
	Relative  bool
	BuildFile string
	Trigger   chan build.Problems
}

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func (pcx *PackageContext) Build(
	ctx context.Context,
	options BuildOptions,
) (
	problems build.Problems,
	result build.Result,
	err error,
) {
	config, err := pcx.buildPrepare(ctx, options.Name, options.Ensure, true)
	if err != nil {
		return
	}

	buildNumber, err := readBuildNumber(options.BuildFile)
	if err != nil {
		return
	}

	command, err := pcx.prepareBuildCommand(ctx, *config)
	if err != nil {
		return
	}
	if options.DryRun {
		printBuildCommand(command)
		return
	}

	return pcx.executeBuild(buildExecutionRequest{
		Context:     ctx,
		Config:      *config,
		Command:     command,
		BuildNumber: buildNumber,
		Options:     options,
	})
}

// BuildWatch runs the Build code on file changes
func (pcx *PackageContext) BuildWatch(ctx context.Context, options BuildOptions) (err error) {
	config, err := pcx.buildPrepare(ctx, options.Name, options.Ensure, true)
	if err != nil {
		return
	}

	buildNumber, err := readBuildNumber(options.BuildFile)
	if err != nil {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to create new filesystem watcher")
	}
	defer func() {
		if closeErr := watcher.Close(); closeErr != nil && err == nil {
			err = errors.Wrap(closeErr, "failed to close filesystem watcher")
		}
	}()

	watchPath := pcx.Package.LocalPath
	if pcx.Package.Entry != "" {
		watchPath = filepath.Dir(packagePath(pcx.Package.LocalPath, pcx.Package.Entry))
	}
	watchPath, err = fs.Abs(watchPath)
	if err != nil {
		return errors.Wrap(err, "failed to resolve build watch path")
	}

	err = filepath.Walk(watchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			print.Warn(err)
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		err = watcher.Add(path)
		if err != nil {
			print.Warn(err)
			return nil
		}

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to add paths to filesystem watcher")
	}

	print.Verb("watching directory for changes", watchPath)

	signals, stopSignals := newTerminationSignals()
	resultCh := make(chan buildWatchResult, 1)
	defer stopSignals()

	var (
		ctxInner, cancel = context.WithCancel(ctx)
		buildRunning     bool
		debouncer        watchDebouncer
	)

	defer func() {
		debouncer.Stop()
		cancel()
	}()

	watcherColour := color.New(color.FgBlack, color.BgGreen).SprintFunc()

	startBuild := func(eventName string) {
		cancel()
		buildRunning = true
		buildRun := atomic.AddUint32(&buildNumber, 1)
		ctxInner, cancel = context.WithCancel(ctx)

		fmt.Printf("%s found modified file: %s\n", watcherColour("WATCHER:"), eventName)
		fmt.Printf("%s compiling %s with compiler version %s [%d]\n", watcherColour("WATCHER:"), config.Input, config.Compiler.Version, buildRun)

		go func(run uint32, changedFile string, buildCtx context.Context) {
			problems, _, buildErr := compiler.CompileSource(buildCtx, compiler.CompileRequest{
				GitHub:   pcx.GitHub,
				ExecDir:  pcx.Package.LocalPath,
				ErrorDir: pcx.Package.LocalPath,
				CacheDir: pcx.CacheDir,
				Platform: pcx.Platform,
				Config:   *config,
				Relative: options.Relative,
			})
			resultCh <- buildWatchResult{
				problems:    problems,
				err:         buildErr,
				eventName:   changedFile,
				buildNumber: run,
			}
		}(buildRun, eventName, ctxInner)
	}

	queueBuild := func(eventName string) {
		debouncer.Queue(eventName, buildWatchDebounce)
	}

	startBuild(pcx.Package.Entry)

loop:
	for {
		select {
		case sig := <-signals:
			fmt.Println("") // insert newline after the ^C
			print.Verb("signal received", sig, "stopping build watcher...")
			cancel()
			break loop
		case errInner := <-watcher.Errors:
			print.Warn("filesystem watcher error:", errInner)

		case event, ok := <-watcher.Events:
			if !ok {
				break loop
			}
			if !shouldWatchBuildEvent(event) {
				continue
			}
			queueBuild(event.Name)

		case <-debouncer.Channel():
			eventName, ok := debouncer.OnTimerFired(buildRunning)
			if !ok {
				continue
			}
			startBuild(eventName)

		case result := <-resultCh:
			buildRunning = false
			cancel()

			if result.err != nil {
				if result.err.Error() == "signal: killed" || result.err.Error() == "context canceled" {
					print.Verb("non-fatal error occurred:", result.err)
				} else {
					print.Erro("Error encountered during build:", errors.Wrapf(result.err, "failed to compile package, run: %d", result.buildNumber))
				}
			} else {
				fmt.Printf("%s finished building: %s [%d]\n", watcherColour("WATCHER:"), result.eventName, result.buildNumber)

				if options.Trigger != nil {
					options.Trigger <- result.problems
				}

				if options.BuildFile != "" {
					err2 := os.WriteFile(options.BuildFile, []byte(fmt.Sprint(result.buildNumber)), 0o700)
					if err2 != nil {
						print.Erro("Failed to write buildfile:", err2)
					}
				}
			}

			if debouncer.Channel() == nil {
				eventName, ok := debouncer.PopPending()
				if ok {
					startBuild(eventName)
				}
			}
		}
	}

	fmt.Printf("%s finished watching all builds\n", watcherColour("WATCHER:"))

	return err
}

func readBuildNumber(buildFile string) (uint32, error) {
	if buildFile == "" {
		return 0, nil
	}
	return readInt(buildFile)
}

func (pcx *PackageContext) prepareBuildCommand(ctx context.Context, config build.Config) (*exec.Cmd, error) {
	return compiler.PrepareCommand(ctx, compiler.PrepareCommandRequest{
		GitHub:   pcx.GitHub,
		ExecDir:  pcx.Package.LocalPath,
		CacheDir: pcx.CacheDir,
		Platform: pcx.Platform,
		Config:   config,
	})
}

func printBuildCommand(command *exec.Cmd) {
	fmt.Println(strings.Join(command.Env, " "), strings.Join(command.Args, " "))
}

type buildExecutionRequest struct {
	Context     context.Context
	Config      build.Config
	Command     *exec.Cmd
	BuildNumber uint32
	Options     BuildOptions
}

func (pcx *PackageContext) executeBuild(request buildExecutionRequest) (problems build.Problems, result build.Result, err error) {
	if err = compiler.RunPreBuildCommands(request.Context, request.Config, os.Stdout); err != nil {
		print.Erro("Failed to execute pre-build command:", err)
		return nil, build.Result{}, err
	}

	print.Verb("building", pcx.Package, "with", request.Config.Compiler.Version)
	problems, result, err = compiler.CompileWithCommand(request.Command, request.Config.WorkingDir, pcx.Package.LocalPath, request.Options.Relative)
	if err != nil {
		return nil, build.Result{}, errors.Wrap(err, "failed to compile package entry")
	}

	atomic.AddUint32(&request.BuildNumber, 1)
	writeBuildNumber(request.Options.BuildFile, request.BuildNumber)

	if err = compiler.RunPostBuildCommands(request.Context, request.Config, os.Stdout); err != nil {
		print.Erro("Failed to execute post-build command:", err)
		return problems, result, err
	}
	return problems, result, nil
}

func writeBuildNumber(buildFile string, buildNumber uint32) {
	if buildFile == "" {
		return
	}
	if err := os.WriteFile(buildFile, []byte(fmt.Sprint(buildNumber)), 0o700); err != nil {
		print.Erro("Failed to write buildfile:", err)
	}
}

func shouldWatchBuildEvent(event fsnotify.Event) bool {
	ext := filepath.Ext(event.Name)
	if ext != ".pwn" && ext != ".inc" {
		return false
	}
	return event.Op&(fsnotify.Write|fsnotify.Create) != 0
}

func (pcx *PackageContext) buildPrepare(
	ctx context.Context,
	build string,
	ensure,
	forceUpdate bool,
) (config *build.Config, err error) {
	config = pcx.Package.GetBuildConfig(build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	if err = config.Compiler.Validate(); err != nil {
		err = errors.Wrap(err, "invalid compiler configuration")
		return
	}

	if config.Input == "" && pcx.Package.Entry != "" {
		config.Input = packagePath(pcx.Package.LocalPath, pcx.Package.Entry)
	} else if config.Input != "" {
		config.Input = packagePath(pcx.Package.LocalPath, config.Input)
	}
	if config.Input != "" {
		config.Input, err = fs.Abs(config.Input)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve build input path")
		}
	}

	if config.WorkingDir == "" {
		switch {
		case config.Input != "":
			config.WorkingDir = filepath.Dir(config.Input)
		case pcx.Package.Entry != "":
			entryPath, absErr := fs.Abs(packagePath(pcx.Package.LocalPath, pcx.Package.Entry))
			if absErr != nil {
				return nil, errors.Wrap(absErr, "failed to resolve package entry path")
			}
			config.WorkingDir = filepath.Dir(entryPath)
		}
	} else {
		config.WorkingDir, err = fs.Abs(packagePath(pcx.Package.LocalPath, config.WorkingDir))
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve build working directory")
		}
	}

	if config.Output == "" && pcx.Package.Output != "" {
		config.Output = packagePath(pcx.Package.LocalPath, pcx.Package.Output)
	} else if config.Output != "" {
		config.Output = packagePath(pcx.Package.LocalPath, config.Output)
	}
	if config.Output != "" {
		config.Output, err = fs.Abs(config.Output)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve build output path")
		}
	}

	if config.Compiler.Path != "" {
		compilerPath := config.Compiler.Path
		if !filepath.IsAbs(compilerPath) {
			compilerPath = filepath.Join(pcx.Package.LocalPath, compilerPath)
		}
		config.Compiler.Path, err = fs.Abs(compilerPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to resolve compiler path")
		}
	}

	config.Includes = append(config.Includes, pcx.Package.LocalPath)

	if err = pcx.ensureBuildFile(config); err != nil {
		return
	}

	if ensure {
		err = pcx.EnsureDependencies(ctx, forceUpdate)
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	for _, depMeta := range pcx.AllDependencies {

		// check if local package has a definition
		incPath := ""
		hasIncludeResources := false
		noPackage := false
		depDir := filepath.Join(pcx.Package.LocalPath, "dependencies", depMeta.Repo)
		pkgInner, errInner := pawnpackage.PackageFromDir(depDir)
		if errInner != nil {
			print.Verb(depMeta, "error while loading:", errInner, "using cached copy for include path checking")
			pkgInner, errInner = pawnpackage.GetCachedPackage(depMeta, pcx.CacheDir)
			if errInner != nil {
				noPackage = true
			}
		}

		if !noPackage {
			// check if package specifies an include path
			if pkgInner.IncludePath != "" {
				incPath = pkgInner.IncludePath
			}
			// check if the package specifies resources that contain includes
			for _, res := range pkgInner.Resources {
				if len(res.Includes) > 0 {
					hasIncludeResources = true
					break
				}
			}
		}

		if !hasIncludeResources {
			config.Includes = append(config.Includes, filepath.Join(depDir, incPath))
		}
	}

	for _, pluginMeta := range pcx.AllPlugins {
		if !pluginMeta.IsLocalScheme() {
			continue
		}
		if pluginMeta.Scheme != "plugin" && pluginMeta.Scheme != "component" {
			continue
		}

		localDepDir := filepath.Join(pcx.Package.LocalPath, pluginMeta.Local)
		if !fs.Exists(localDepDir) {
			err = errors.Errorf("local %s dependency path does not exist: %s", pluginMeta.Scheme, localDepDir)
			return
		}

		includeDir := localDepDir
		if pkgInner, errInner := pawnpackage.PackageFromDir(localDepDir); errInner == nil {
			if pkgInner.IncludePath != "" {
				includeDir = filepath.Join(localDepDir, pkgInner.IncludePath)
			}
		}

		config.Includes = append(config.Includes, includeDir)
	}

	config.Includes = append(config.Includes, pcx.AllIncludePaths...)

	return config, err
}

func (pcx *PackageContext) ensureBuildFile(config *build.Config) (err error) {
	exp := pcx.Package.ExperimentalFlags()
	if exp == nil || !exp.BuildFileEnabled() {
		return nil
	}

	buildFilePath := filepath.Join(pcx.Package.LocalPath, "sampctl_build_file.inc")
	var builder strings.Builder
	builder.WriteString("// Code generated by sampctl. DO NOT EDIT.\n")
	builder.WriteString("#if defined _sampctl_build_file_included\n")
	builder.WriteString("  #endinput\n")
	builder.WriteString("#endif\n")
	builder.WriteString("#define _sampctl_build_file_included\n\n")

	writeDefine := func(name, value string, quoted bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if quoted {
			escaped := strings.ReplaceAll(value, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
			escaped = strings.ReplaceAll(escaped, "\n", "\\n")
			escaped = strings.ReplaceAll(escaped, "\r", "\\r")
			escaped = strings.ReplaceAll(escaped, "\t", "\\t")
			builder.WriteString("#define " + name + " \"" + escaped + "\"\n")
			return
		}
		builder.WriteString("#define " + name + " " + value + "\n")
	}

	isNumericLiteral := func(value string) bool {
		v := strings.TrimSpace(value)
		if v == "" {
			return false
		}
		if v[0] == '-' {
			v = v[1:]
			if v == "" {
				return false
			}
		}
		dotSeen := false
		for _, r := range v {
			if r == '.' {
				if dotSeen {
					return false
				}
				dotSeen = true
				continue
			}
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}

	readGitInfo := func(repoPath string) (commit, branch string) {
		headPath := filepath.Join(repoPath, ".git", "HEAD")
		headBytes, err := os.ReadFile(headPath)
		if err != nil {
			return "", ""
		}
		head := strings.TrimSpace(string(headBytes))
		if strings.HasPrefix(head, "ref: ") {
			ref := strings.TrimSpace(strings.TrimPrefix(head, "ref: "))
			refPath := filepath.Join(repoPath, ".git", filepath.FromSlash(ref))
			refBytes, refErr := os.ReadFile(refPath)
			if refErr == nil {
				commit = strings.TrimSpace(string(refBytes))
			}
			branch = filepath.Base(ref)
			return commit, branch
		}
		if head != "" {
			commit = head
		}
		return commit, branch
	}

	if _, exists := config.Constants["SAMPCTL_BUILD_FILE"]; !exists {
		writeDefine("SAMPCTL_BUILD_FILE", "1", false)
	}
	if _, exists := config.Constants["SAMPCTL_VERSION"]; !exists {
		version := strings.TrimSpace(pcx.AppVersion)
		if version == "" {
			version = "unknown"
		}
		writeDefine("SAMPCTL_VERSION", version, true)
	}
	if pcx.Platform != "" {
		if _, exists := config.Constants["SAMPCTL_PLATFORM"]; !exists {
			writeDefine("SAMPCTL_PLATFORM", pcx.Platform, true)
		}
	}
	builder.WriteString("\n")

	if commit, branch := readGitInfo(pcx.Package.LocalPath); commit != "" {
		if _, exists := config.Constants["SAMPCTL_BUILD_COMMIT"]; !exists {
			writeDefine("SAMPCTL_BUILD_COMMIT", commit, true)
		}
		if _, exists := config.Constants["SAMPCTL_BUILD_COMMIT_SHORT"]; !exists {
			commitShort := commit
			if len(commitShort) > 7 {
				commitShort = commitShort[:7]
			}
			writeDefine("SAMPCTL_BUILD_COMMIT_SHORT", commitShort, true)
		}
		if branch != "" {
			if _, exists := config.Constants["SAMPCTL_BUILD_BRANCH"]; !exists {
				writeDefine("SAMPCTL_BUILD_BRANCH", branch, true)
			}
		}
		builder.WriteString("\n")
	}

	resolveConstantValue := func(value string) string {
		if strings.HasPrefix(value, "$") {
			return os.Getenv(value[1:])
		}
		return value
	}

	keys := make([]string, 0, len(config.Constants))
	for name := range config.Constants {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		value := strings.TrimSpace(resolveConstantValue(config.Constants[name]))
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			writeDefine(name, value, false)
			continue
		}
		if isNumericLiteral(value) {
			writeDefine(name, value, false)
		} else {
			writeDefine(name, value, true)
		}
	}
	builder.WriteString("\n")

	if err = os.WriteFile(buildFilePath, []byte(builder.String()), 0o600); err != nil {
		return errors.Wrap(err, "failed to write build include file")
	}

	return nil
}

func readInt(file string) (n uint32, err error) {
	var contents []byte
	if fs.Exists(file) {
		contents, err = os.ReadFile(file)
		if err != nil {
			err = errors.Wrap(err, "failed to read buildfile")
			return
		}
		var result int
		result, err = strconv.Atoi(string(contents))
		if err != nil {
			err = errors.Wrap(err, "failed to interpret buildfile contents as an integer number")
			return
		}
		if result < 0 {
			err = errors.Wrap(err, "build number is not a positive integer")
			return
		}
		n = uint32(result)
	} else {
		err = os.WriteFile(file, []byte("0"), 0o700)
		n = 0
	}
	return
}
