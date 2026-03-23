package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
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

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func (pcx *PackageContext) Build(
	ctx context.Context,
	build string,
	ensure bool,
	dry bool,
	relative bool,
	buildFile string,
) (
	problems build.Problems,
	result build.Result,
	err error,
) {
	config, err := pcx.buildPrepare(ctx, build, ensure, true)
	if err != nil {
		return
	}

	buildNumber := uint32(0)
	if buildFile != "" {
		buildNumber, err = readInt(buildFile)
		if err != nil {
			return
		}
	}

	command, err := compiler.PrepareCommand(
		ctx,
		pcx.GitHub,
		pcx.Package.LocalPath,
		pcx.CacheDir,
		pcx.Platform,
		*config,
	)
	if err != nil {
		return
	}

	if dry {
		fmt.Println(strings.Join(command.Env, " "), strings.Join(command.Args, " "))
	} else {
		print.Verb("running pre-build commands")
		err = compiler.RunPreBuildCommands(ctx, *config, os.Stdout)
		if err != nil {
			print.Erro("Failed to execute pre-build command: ", err)
			return
		}

		print.Verb("building", pcx.Package, "with", config.Compiler.Version)

		problems, result, err = compiler.CompileWithCommand(
			command,
			config.WorkingDir,
			pcx.Package.LocalPath,
			relative,
		)
		if err != nil {
			err = errors.Wrap(err, "failed to compile package entry")
			return
		}

		atomic.AddUint32(&buildNumber, 1)

		if buildFile != "" {
			err2 := os.WriteFile(buildFile, []byte(fmt.Sprint(buildNumber)), 0o700)
			if err2 != nil {
				print.Erro("Failed to write buildfile:", err2)
			}
		}

		print.Verb("running post-build commands")
		err = compiler.RunPostBuildCommands(ctx, *config, os.Stdout)
		if err != nil {
			print.Erro("Failed to execute post-build command: ", err)
			return
		}
	}

	return problems, result, err
}

// BuildWatch runs the Build code on file changes
func (pcx *PackageContext) BuildWatch(
	ctx context.Context,
	name string,
	ensure bool,
	buildFile string,
	relative bool,
	trigger chan build.Problems,
) (err error) {
	config, err := pcx.buildPrepare(ctx, name, ensure, true)
	if err != nil {
		return
	}

	buildNumber := uint32(0)
	if buildFile != "" {
		buildNumber, err = readInt(buildFile)
		if err != nil {
			return
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to create new filesystem watcher")
	}
	defer watcher.Close()

	watchPath := path.Dir(pcx.Package.Entry)
	if watchPath == "" {
		watchPath = pcx.Package.LocalPath
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

	signals := make(chan os.Signal, 1)
	resultCh := make(chan buildWatchResult, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	var (
		ctxInner, cancel = context.WithCancel(ctx)
		buildRunning     bool
		pendingEvent     string
		debounceTimer    *time.Timer
		debounceCh       <-chan time.Time
	)

	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		cancel()
	}()

	watcherColour := color.New(color.FgBlack, color.BgGreen).SprintFunc()

	startBuild := func(eventName string) {
		cancel()
		buildRunning = true
		pendingEvent = ""
		buildRun := atomic.AddUint32(&buildNumber, 1)
		ctxInner, cancel = context.WithCancel(ctx)

		fmt.Printf("%s found modified file: %s\n", watcherColour("WATCHER:"), eventName)
		fmt.Printf("%s compiling %s with compiler version %s [%d]\n", watcherColour("WATCHER:"), config.Input, config.Compiler.Version, buildRun)

		go func(run uint32, changedFile string, buildCtx context.Context) {
			problems, _, buildErr := compiler.CompileSource(
				buildCtx,
				pcx.GitHub,
				pcx.Package.LocalPath,
				pcx.Package.LocalPath,
				pcx.CacheDir,
				pcx.Platform,
				*config,
				relative,
			)
			resultCh <- buildWatchResult{
				problems:    problems,
				err:         buildErr,
				eventName:   changedFile,
				buildNumber: run,
			}
		}(buildRun, eventName, ctxInner)
	}

	queueBuild := func(eventName string) {
		pendingEvent = eventName
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(buildWatchDebounce)
			debounceCh = debounceTimer.C
			return
		}

		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(buildWatchDebounce)
		debounceCh = debounceTimer.C
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

		case <-debounceCh:
			debounceCh = nil
			if pendingEvent == "" || buildRunning {
				continue
			}
			startBuild(pendingEvent)

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

				if trigger != nil {
					trigger <- result.problems
				}

				if buildFile != "" {
					err2 := os.WriteFile(buildFile, []byte(fmt.Sprint(result.buildNumber)), 0o700)
					if err2 != nil {
						print.Erro("Failed to write buildfile:", err2)
					}
				}
			}

			if pendingEvent != "" && debounceCh == nil {
				startBuild(pendingEvent)
			}
		}
	}

	fmt.Printf("%s finished watching all builds\n", watcherColour("WATCHER:"))

	return err
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

	if config.WorkingDir == "" {
		entryPath, absErr := fs.Abs(pcx.Package.Entry)
		if absErr != nil {
			return nil, errors.Wrap(absErr, "failed to resolve package entry path")
		}
		config.WorkingDir = filepath.Dir(entryPath)
	}
	if config.Input == "" {
		config.Input = filepath.Join(pcx.Package.LocalPath, pcx.Package.Entry)
	}
	if config.Output == "" {
		config.Output = filepath.Join(pcx.Package.LocalPath, pcx.Package.Output)
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
	if exp == nil || !exp.BuildFile {
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
