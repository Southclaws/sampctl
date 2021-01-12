package pkgcontext

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/build"
	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/pawnpackage"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
)

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

	var buildNumber = uint32(0)
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
		}

		atomic.AddUint32(&buildNumber, 1)

		if buildFile != "" {
			err2 := ioutil.WriteFile(buildFile, []byte(fmt.Sprint(buildNumber)), 0755)
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

	var buildNumber = uint32(0)
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

	path := path.Dir(pcx.Package.Entry)
	if path == "" {
		path = pcx.Package.LocalPath
	}

	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
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

	print.Verb("watching directory for changes", path)

	signals := make(chan os.Signal, 1)
	errorCh := make(chan error)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var (
		running          atomic.Value
		ctxInner, cancel = context.WithCancel(ctx)
		problems         []build.Problem
		lastEvent        time.Time
	)

	defer cancel()

	running.Store(false)

	watcherColour := color.New(color.FgBlack, color.BgGreen).SprintFunc()

	// send a fake first event to trigger an initial build
	go func() { watcher.Events <- fsnotify.Event{Name: pcx.Package.Entry, Op: fsnotify.Write} }()

loop:
	for {
		select {
		case sig := <-signals:
			fmt.Println("") // insert newline after the ^C
			print.Verb("signal received", sig, "stopping build watcher...")
			break loop
		case errInner := <-errorCh:
			print.Erro("Error encountered during build:", errInner)
			break loop

		case event := <-watcher.Events:
			ext := filepath.Ext(event.Name)
			if ext != ".pwn" && ext != ".inc" {
				continue
			}
			if event.Op != fsnotify.Write && event.Op != fsnotify.Create {
				continue
			}

			if time.Since(lastEvent) < time.Millisecond*500 {
				print.Verb("skipping duplicate write", time.Since(lastEvent), "since last file change")
				continue
			}
			lastEvent = time.Now()

			go func() {
				if running.Load().(bool) {
					print.Verb("Build interrupted by file change")
					cancel()
					ctxInner, cancel = context.WithCancel(ctx)
				}

				atomic.AddUint32(&buildNumber, 1)
				fmt.Printf("%s found modified file: %s\n", watcherColour("WATCHER:"), event.Name)
				fmt.Printf("%s compiling %s with compiler version %s [%d]\n", watcherColour("WATCHER:"), config.Input, config.Compiler.Version, buildNumber)

				running.Store(true)
				problems, _, err = compiler.CompileSource(
					ctxInner,
					pcx.GitHub,
					pcx.Package.LocalPath,
					pcx.Package.LocalPath,
					pcx.CacheDir,
					pcx.Platform,
					*config,
					relative,
				)
				running.Store(false)

				if err != nil {
					if err.Error() == "signal: killed" || err.Error() == "context canceled" {
						print.Verb("non-fatal error occurred:", err)
						return
					}

					errorCh <- errors.Wrapf(err, "failed to compile package, run: %d", buildNumber)
				}
				fmt.Printf("%s finished building: %s [%d]\n", watcherColour("WATCHER:"), event.Name, buildNumber)

				if trigger != nil {
					trigger <- problems
				}

				if buildFile != "" {
					err2 := ioutil.WriteFile(buildFile, []byte(fmt.Sprint(buildNumber)), 0755)
					if err2 != nil {
						print.Erro("Failed to write buildfile:", err2)
					}
				}
			}()
		}
	}

	fmt.Printf("%s finished watching all builds\n", watcherColour("WATCHER:"))

	return err
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

	if config.WorkingDir == "" {
		config.WorkingDir = filepath.Dir(util.FullPath(pcx.Package.Entry))
	}
	if config.Input == "" {
		config.Input = filepath.Join(pcx.Package.LocalPath, pcx.Package.Entry)
	}
	if config.Output == "" {
		config.Output = filepath.Join(pcx.Package.LocalPath, pcx.Package.Output)
	}

	config.Includes = []string{pcx.Package.LocalPath}

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

	config.Includes = append(config.Includes, pcx.AllIncludePaths...)

	return config, err
}

func readInt(file string) (n uint32, err error) {
	var contents []byte
	if util.Exists(file) {
		contents, err = ioutil.ReadFile(file)
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
		err = ioutil.WriteFile(file, []byte("0"), 0755)
		n = 0
	}
	return
}
