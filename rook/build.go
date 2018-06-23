package rook

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func Build(ctx context.Context, gh *github.Client, auth transport.AuthMethod, pkg *types.Package, build, cacheDir, platform string, ensure, dry bool, relative bool, buildFile string) (problems types.BuildProblems, result types.BuildResult, err error) {
	config := GetBuildConfig(*pkg, build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	config.WorkingDir = filepath.Dir(util.FullPath(pkg.Entry))
	config.Input = filepath.Join(pkg.LocalPath, pkg.Entry)
	config.Output = filepath.Join(pkg.LocalPath, pkg.Output)

	if ensure {
		err = EnsureDependencies(ctx, gh, pkg, auth, platform, cacheDir)
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	var buildNumber = uint32(0)
	if buildFile != "" {
		buildNumber, err = readInt(buildFile)
		if err != nil {
			return
		}
	}

	// TODO: figure out why this was here in the first place
	// then figure out a better solution...
	// print.Verb(pkg, "resolving dependencies before build")
	// err = ResolveDependencies(pkg, platform)
	// if err != nil {
	// 	err = errors.Wrap(err, "failed to resolve dependencies before build")
	// 	return
	// }

	var pkgInner types.Package
	for _, depMeta := range pkg.AllDependencies {
		depDir := filepath.Join(pkg.LocalPath, "dependencies", depMeta.Repo)
		incPath := depMeta.Path

		// check if local package has a definition, if so, check if it has an IncludePath field
		pkgInner, err = types.PackageFromDir(depDir)
		if err == nil {
			if pkgInner.IncludePath != "" {
				incPath = pkgInner.IncludePath
			}
		}

		config.Includes = append(config.Includes, filepath.Join(depDir, incPath))
	}

	config.Includes = append(config.Includes, pkg.AllIncludePaths...)

	command, err := compiler.PrepareCommand(ctx, gh, pkg.LocalPath, cacheDir, platform, *config)
	if err != nil {
		return
	}

	if dry {
		fmt.Println(strings.Join(command.Env, " "), strings.Join(command.Args, " "))
	} else {
		for _, plugin := range config.Plugins {
			print.Verb("running pre-build plugin", plugin)
			pluginCmd := exec.Command(plugin[0], plugin[1:]...)
			pluginCmd.Stdout = os.Stdout
			pluginCmd.Stderr = os.Stdout
			err = pluginCmd.Run()
			if err != nil {
				print.Erro("Failed to execute pre-build plugin:", plugin[0], err)
				return
			}
		}
		print.Verb("building", pkg, "with", config.Version)

		problems, result, err = compiler.CompileWithCommand(command, config.WorkingDir, pkg.LocalPath, relative)
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
	}

	return
}

// BuildWatch runs the Build code on file changes
func BuildWatch(ctx context.Context, gh *github.Client, auth transport.AuthMethod, pkg *types.Package, build, cacheDir, platform string, ensure bool, buildFile string, relative bool, trigger chan types.BuildProblems) (err error) {
	config := GetBuildConfig(*pkg, build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	config.WorkingDir = filepath.Dir(util.FullPath(pkg.Entry))
	config.Input = filepath.Join(pkg.LocalPath, pkg.Entry)
	config.Output = filepath.Join(pkg.LocalPath, pkg.Output)

	if ensure {
		err = EnsureDependencies(ctx, gh, pkg, auth, platform, cacheDir)
		if err != nil {
			err = errors.Wrap(err, "failed to ensure dependencies before build")
			return
		}
	}

	var buildNumber = uint32(0)
	if buildFile != "" {
		buildNumber, err = readInt(buildFile)
		if err != nil {
			return
		}
	}

	// TODO: move these things out of the builders and into a prep func for DRY
	// print.Verb(pkg, "resolving dependencies before build watcher")
	// err = ResolveDependencies(pkg, platform)
	// if err != nil {
	// 	err = errors.Wrap(err, "failed to resolve dependencies before build watcher")
	// 	return
	// }

	var pkgInner types.Package
	for _, depMeta := range pkg.AllDependencies {
		depDir := filepath.Join(pkg.LocalPath, "dependencies", depMeta.Repo)
		incPath := depMeta.Path

		// check if local package has a definition, if so, check if it has an IncludePath field
		pkgInner, err = types.PackageFromDir(depDir)
		if err == nil {
			if pkgInner.IncludePath != "" {
				incPath = pkgInner.IncludePath
			}
		}

		config.Includes = append(config.Includes, filepath.Join(depDir, incPath))
	}

	config.Includes = append(config.Includes, pkg.AllIncludePaths...)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to create new filesystem watcher")
	}
	err = filepath.Walk(pkg.LocalPath, func(path string, info os.FileInfo, err error) error {
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

	print.Verb("watching directory for changes", pkg.LocalPath)

	signals := make(chan os.Signal, 1)
	errorCh := make(chan error)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var (
		running          atomic.Value
		ctxInner, cancel = context.WithCancel(ctx)
		problems         []types.BuildProblem
		lastEvent        time.Time
	)

	defer func() {
		print.Warn("cancelled inner context")
		cancel()
	}()
	running.Store(false)

	// send a fake first event to trigger an initial build
	go func() { watcher.Events <- fsnotify.Event{Name: pkg.Entry, Op: fsnotify.Write} }()

loop:
	for {
		select {
		case sig := <-signals:
			fmt.Println("") // insert newline after the ^C
			print.Info("signal received", sig, "stopping build watcher...")
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
					fmt.Println("watch-build: killing existing compiler process")
					cancel()
					fmt.Println("watch-build: killed existing compiler process")
					// re-create context and canceler
					ctxInner, cancel = context.WithCancel(ctx)
					defer func() {
						print.Verb("cancelling existing compiler execution context")
						cancel()
					}()
				}

				atomic.AddUint32(&buildNumber, 1)
				fmt.Println("watch-build: starting compilation", buildNumber)

				running.Store(true)
				problems, _, err = compiler.CompileSource(ctxInner, gh, pkg.LocalPath, pkg.LocalPath, cacheDir, platform, *config, relative)
				running.Store(false)

				if err != nil {
					if err.Error() == "signal: killed" || err.Error() == "context canceled" {
						print.Erro("non-fatal error occurred:", err)
						return
					}

					errorCh <- errors.Wrapf(err, "failed to compile package, run: %d", buildNumber)
				}
				fmt.Println("watch-build: finished", buildNumber)

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

	print.Info("finished running build watcher")

	return
}

// GetBuildConfig returns a matching build by name from the package build list. If no name is
// specified, the first build is returned. If the package has no build definitions, a default
// configuration is returned.
func GetBuildConfig(pkg types.Package, name string) (config *types.BuildConfig) {
	def := types.GetBuildConfigDefault()

	// if there are no builds at all, use default
	if len(pkg.Builds) == 0 && pkg.Build == nil {
		return def
	}

	// if the user did not specify a specific build config, use the first
	// otherwise, search for a matching config by name
	if name == "default" {
		if pkg.Build != nil {
			config = pkg.Build
		} else {
			config = pkg.Builds[0]
		}
	} else {
		for _, cfg := range pkg.Builds {
			if cfg.Name == name {
				config = cfg
				break
			}
		}
	}

	if config == nil {
		print.Warn("No build config called:", name, "using default")
		return def
	}

	if config.Version == "" {
		config.Version = def.Version
	}
	if len(config.Args) == 0 {
		config.Args = def.Args
	}

	return
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
