//go:build !windows

package pkgcontext

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
)

const (
	watchSignalHelperEnv = "SAMPCTL_WATCH_SIGNAL_HELPER"
	watchSignalReady     = "watch-signal-ready"
)

func TestWatchersHandleSIGINT(t *testing.T) {
	for _, mode := range []string{"buffered", "unbuffered", "run"} {
		t.Run(mode, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestWatchSignalHelper$")
			command.Env = append(os.Environ(), watchSignalHelperEnv+"="+mode)
			stdout, err := command.StdoutPipe()
			require.NoError(t, err)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			require.NoError(t, command.Start())
			defer func() { _ = command.Process.Kill() }()

			ready := make(chan struct{}, 1)
			finished := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				var output strings.Builder
				for scanner.Scan() {
					line := scanner.Text()
					output.WriteString(line + "\n")
					if line == watchSignalReady {
						ready <- struct{}{}
					}
				}
				waitErr := command.Wait()
				if waitErr != nil || scanner.Err() != nil {
					finished <- fmt.Errorf("watcher helper failed: wait=%v, scan=%v\n%s%s", waitErr, scanner.Err(), output.String(), stderr.String())
					return
				}
				finished <- nil
			}()

			select {
			case <-ready:
			case err := <-finished:
				t.Fatalf("watcher helper exited before readiness: %v", err)
			case <-time.After(10 * time.Second):
				t.Fatal("watcher helper did not become ready")
			}
			require.NoError(t, command.Process.Signal(syscall.SIGINT))

			select {
			case err := <-finished:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Fatal("watcher helper did not stop after SIGINT")
			}
		})
	}
}

func TestWatchSignalHelper(t *testing.T) {
	mode := os.Getenv(watchSignalHelperEnv)
	if mode == "" {
		return
	}

	pcx := newRunBuildFixture(t)
	done := make(chan error, 1)
	switch mode {
	case "run":
		env := &blockingRunEnvironment{started: make(chan struct{}), stopped: make(chan struct{})}
		pcx.RuntimeEnv = env
		go func() { done <- pcx.RunWatch(t.Context()) }()
		select {
		case <-env.started:
		case err := <-done:
			t.Fatalf("run watcher exited before runtime started: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("watched runtime did not start")
		}

	case "buffered", "unbuffered":
		var outputEvents <-chan fsnotify.Event
		capacity := 1
		if mode == "unbuffered" {
			capacity = 0
			buildDir := filepath.Join(pcx.Package.LocalPath, "build")
			require.NoError(t, os.Mkdir(buildDir, 0o755))
			watcher, err := fsnotify.NewWatcher()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, watcher.Close()) })
			require.NoError(t, watcher.Add(buildDir))
			outputEvents = watcher.Events
		}

		trigger := make(chan build.Problems, capacity)
		var firstBuild <-chan build.Problems
		if capacity > 0 {
			firstBuild = trigger
		}
		go func() { done <- pcx.BuildWatch(t.Context(), BuildOptions{Name: "release", Trigger: trigger}) }()
		select {
		case <-firstBuild:
		case <-outputEvents:
		case err := <-done:
			t.Fatalf("build watcher exited before readiness: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("watched compiler did not complete its output")
		}

	default:
		t.Fatalf("unknown watcher helper mode %q", mode)
	}

	_, err := fmt.Fprintln(os.Stdout, watchSignalReady)
	require.NoError(t, err)
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher ignored SIGINT")
	}
}
