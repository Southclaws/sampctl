package rook

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Build compiles a package, dependencies are ensured and a list of paths are sent to the compiler.
func Build(pkg *types.Package, build, cacheDir, platform string, ensure bool, buildFile string) (problems []types.BuildProblem, result types.BuildResult, err error) {
	config := GetBuildConfig(*pkg, build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	config.WorkingDir = filepath.Dir(util.FullPath(pkg.Entry))
	config.Input = filepath.Join(pkg.Local, pkg.Entry)
	config.Output = filepath.Join(pkg.Local, pkg.Output)

	if ensure {
		err = EnsureDependencies(pkg)
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

	print.Verb(pkg, "resolving dependencies before build")
	err = ResolveDependencies(pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to resolve dependencies before build")
		return
	}

	for _, depMeta := range pkg.AllDependencies {
		depDir := filepath.Join(pkg.Local, "dependencies", depMeta.Repo)
		incPath := depMeta.Path

		// check if local package has a definition, if so, check if it has an IncludePath field
		pkg, err := types.PackageFromDir(depDir)
		if err == nil {
			if pkg.IncludePath != "" {
				incPath = pkg.IncludePath
			}
		}

		config.Includes = append(config.Includes, filepath.Join(depDir, incPath))
	}

	print.Verb("building", pkg, "with", config.Version)

	problems, result, err = compiler.CompileSource(context.Background(), pkg.Local, cacheDir, platform, *config)
	if err != nil {
		err = errors.Wrap(err, "failed to compile package entry")
	}

	if buildFile != "" {
		err2 := ioutil.WriteFile(buildFile, []byte(fmt.Sprint(buildNumber)), 0755)
		if err2 != nil {
			print.Erro("Failed to write buildfile:", err2)
		}
	}

	return
}

// BuildWatch runs the Build code on file changes
func BuildWatch(ctx context.Context, pkg *types.Package, build, cacheDir, platform string, ensure bool, buildFile string, trigger chan []types.BuildProblem) (err error) {
	config := GetBuildConfig(*pkg, build)
	if config == nil {
		err = errors.Errorf("no build config named '%s'", build)
		return
	}

	config.WorkingDir = filepath.Dir(util.FullPath(pkg.Entry))
	config.Input = filepath.Join(pkg.Local, pkg.Entry)
	config.Output = filepath.Join(pkg.Local, pkg.Output)

	if ensure {
		err = EnsureDependencies(pkg)
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

	print.Verb(pkg, "resolving dependencies before build watcher")
	err = ResolveDependencies(pkg)
	if err != nil {
		err = errors.Wrap(err, "failed to resolve dependencies before build watcher")
		return
	}

	for _, depMeta := range pkg.AllDependencies {
		depDir := filepath.Join(pkg.Local, "dependencies", depMeta.Repo)
		incPath := depMeta.Path

		// check if local package has a definition, if so, check if it has an IncludePath field
		pkg, err := types.PackageFromDir(depDir)
		if err == nil {
			if pkg.IncludePath != "" {
				incPath = pkg.IncludePath
			}
		}

		config.Includes = append(config.Includes, filepath.Join(depDir, incPath))
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "failed to create new filesystem watcher")
	}
	err = watcher.Add(pkg.Local)
	if err != nil {
		return errors.Wrap(err, "failed to add package directory to filesystem watcher")
	}

	print.Verb("watching directory for changes", pkg.Local)

	signals := make(chan os.Signal, 1)
	errorCh := make(chan error)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	var (
		running          atomic.Value
		ctxInner, cancel = context.WithCancel(ctx)
		problems         []types.BuildProblem
	)

	running.Store(false)

loop:
	for {
		select {
		case sig := <-signals:
			fmt.Println("") // insert newline after the ^C
			print.Info("signal received", sig, "stopping build watcher...")
			break loop
		case err := <-errorCh:
			print.Erro("Error encountered during build:", err)
			break loop

		case event := <-watcher.Events:
			ext := filepath.Ext(event.Name)
			if ext != ".pwn" && ext != ".inc" {
				continue
			}
			if event.Op != fsnotify.Write && event.Op != fsnotify.Create {
				continue
			}

			if running.Load().(bool) {
				fmt.Println("watch-build: killing existing compiler process", buildNumber)
				cancel()
				fmt.Println("watch-build: finished", buildNumber)
				// re-create context and canceler
				ctxInner, cancel = context.WithCancel(context.Background())
			}

			atomic.AddUint32(&buildNumber, 1)

			fmt.Println("watch-build: starting compilation", buildNumber)
			go func() {
				running.Store(true)
				problems, _, err = compiler.CompileSource(ctxInner, pkg.Local, cacheDir, platform, *config)
				running.Store(false)

				if trigger != nil {
					trigger <- problems
				}

				if err != nil {
					if err.Error() == "signal: killed" || err.Error() == "context canceled" {
						return
					}

					errorCh <- errors.Wrapf(err, "failed to compile package, run: %d", buildNumber)
				}
				fmt.Println("watch-build: finished", buildNumber)

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

	if len(pkg.Builds) == 0 {
		config = def
	} else {
		if name == "" {
			config = &pkg.Builds[0]
		} else {
			for _, cfg := range pkg.Builds {
				if cfg.Name == name {
					config = &cfg
				}
			}
		}
		if config.Version == "" {
			config.Version = def.Version
		}
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
