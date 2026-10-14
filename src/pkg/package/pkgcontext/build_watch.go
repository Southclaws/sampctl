package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

const buildWatchDebounce = 750 * time.Millisecond

type buildWatchResult struct {
	problems    build.Problems
	err         error
	eventName   string
	buildNumber uint32
}

// BuildWatch runs the Build code on file changes
func (pcx *PackageContext) BuildWatch(ctx context.Context, options BuildOptions) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

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
		builds           sync.WaitGroup
	)

	defer func() {
		debouncer.Stop()
		cancel()
		builds.Wait()
	}()

	watcherColour := color.New(color.FgBlack, color.BgGreen).SprintFunc()

	startBuild := func(eventName string) {
		cancel()
		buildRunning = true
		buildRun := atomic.AddUint32(&buildNumber, 1)
		ctxInner, cancel = context.WithCancel(ctx)

		fmt.Printf("%s found modified file: %s\n", watcherColour("WATCHER:"), eventName)
		fmt.Printf("%s compiling %s with compiler version %s [%d]\n", watcherColour("WATCHER:"), config.Input, config.Compiler.Version, buildRun)

		builds.Add(1)
		go func(run uint32, changedFile string, buildCtx context.Context) {
			defer builds.Done()

			problems, _, buildErr := compiler.CompileSource(buildCtx, compiler.CompileRequest{
				GitHub:   pcx.GitHub,
				ExecDir:  pcx.Package.LocalPath,
				ErrorDir: pcx.Package.LocalPath,
				CacheDir: pcx.CacheDir,
				Platform: pcx.Platform,
				Config:   *config,
				Relative: options.Relative,
			})
			result := buildWatchResult{
				problems:    problems,
				err:         buildErr,
				eventName:   changedFile,
				buildNumber: run,
			}
			select {
			case resultCh <- result:
			case <-buildCtx.Done():
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
		case <-ctx.Done():
			err = ctx.Err()
			break loop

		case sig := <-signals:
			fmt.Println("") // insert newline after the ^C
			print.Verb("signal received", sig, "stopping build watcher...")
			err = errors.Wrapf(context.Canceled, "received signal: %v", sig)
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
					print.Verb("Build interrupted:", result.err)
				} else {
					print.Erro("Build failed:", errors.Wrapf(result.err, "failed to compile package, run: %d", result.buildNumber))
				}
			} else {
				fmt.Printf("%s finished building: %s [%d]\n", watcherColour("WATCHER:"), result.eventName, result.buildNumber)

				if options.Trigger != nil {
					select {
					case options.Trigger <- result.problems:
					case <-ctx.Done():
						err = ctx.Err()
						break loop
					case sig := <-signals:
						err = errors.Wrapf(context.Canceled, "received signal: %v", sig)
						break loop
					}
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

func shouldWatchBuildEvent(event fsnotify.Event) bool {
	ext := filepath.Ext(event.Name)
	if ext != ".pwn" && ext != ".inc" {
		return false
	}
	return event.Op&(fsnotify.Write|fsnotify.Create) != 0
}
