package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestRunPrepareStagesSelectedBuildOutput(t *testing.T) {
	pcx := newRunBuildFixture(t)
	pcx.ForceBuild = true

	require.NoError(t, pcx.RunPrepare(t.Context()))
	require.Equal(t, []string{"release"}, pcx.ActualRuntime.Gamemodes)
	contents, err := os.ReadFile(filepath.Join(pcx.Package.LocalPath, "server", "gamemodes", "release.amx"))
	require.NoError(t, err)
	require.Equal(t, "new release", string(contents))

	output, err := pcx.runtimeOutputPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(pcx.Package.LocalPath, "build", "release.amx"), output)
}

func TestContainerRunOptionsPreserveSelectedConfiguration(t *testing.T) {
	pcx := newRunBuildFixture(t)
	pcx.Runtime = "development"
	pcx.ForceBuild, pcx.ForceEnsure, pcx.NoCache, pcx.Relative = true, true, true, true
	pcx.BuildFile = filepath.Join(pcx.Package.LocalPath, "build-number")

	options := pcx.runtimeRunOptions(nil, nil, false)
	require.Equal(t, pcx.Package.LocalPath, options.ProjectDir)
	require.Equal(t, []string{
		"--dir", "/samp", "--build", "release", "--forceBuild", "--forceEnsure", "--noCache", "--relativePaths",
		"--buildFile", "/samp/build-number", "development",
	}, options.Args)
}

func TestBuildWatchReturnsAfterCancellation(t *testing.T) {
	for _, buffered := range []bool{true, false} {
		name := "waiting for trigger consumer"
		if buffered {
			name = "after completed build"
		}
		t.Run(name, func(t *testing.T) {
			pcx := newRunBuildFixture(t)
			var outputEvents <-chan fsnotify.Event
			if !buffered {
				buildDir := filepath.Join(pcx.Package.LocalPath, "build")
				require.NoError(t, os.Mkdir(buildDir, 0o755))
				watcher, err := fsnotify.NewWatcher()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, watcher.Close()) })
				require.NoError(t, watcher.Add(buildDir))
				outputEvents = watcher.Events
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			capacity := 0
			if buffered {
				capacity = 1
			}
			trigger := make(chan build.Problems, capacity)
			done := make(chan error, 1)
			go func() { done <- pcx.BuildWatch(ctx, BuildOptions{Name: "release", Trigger: trigger}) }()

			if buffered {
				select {
				case <-trigger:
				case err := <-done:
					t.Fatalf("watcher exited before its first build: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("initial watched build did not complete")
				}
			} else {
				select {
				case <-outputEvents:
				case err := <-done:
					t.Fatalf("watcher exited before writing build output: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("watched compiler did not write output")
				}
			}
			cancel()

			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("build watcher ignored cancellation")
			}
		})
	}
}

type blockingRunEnvironment struct {
	fakeRuntimeEnvironment
	started chan struct{}
	stopped chan struct{}
}

func (env *blockingRunEnvironment) Run(ctx context.Context, _ run.Runtime, _ runtimepkg.RunOptions) error {
	close(env.started)
	defer close(env.stopped)
	<-ctx.Done()
	return ctx.Err()
}

func TestRunWatchCancelsAndJoinsRuntime(t *testing.T) {
	pcx := newRunBuildFixture(t)
	env := &blockingRunEnvironment{started: make(chan struct{}), stopped: make(chan struct{})}
	pcx.RuntimeEnv = env
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- pcx.RunWatch(ctx) }()

	select {
	case <-env.started:
	case err := <-done:
		t.Fatalf("run watcher exited before starting runtime: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("watched runtime did not start")
	}
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		select {
		case <-env.stopped:
		default:
			t.Fatal("run watcher returned before runtime stopped")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run watcher ignored cancellation")
	}
}

func newRunBuildFixture(t *testing.T) *PackageContext {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture compiler is a POSIX shell script")
	}

	project := t.TempDir()
	for relative, data := range map[string]string{
		"gamemodes/main.pwn":    "main() {}",
		"gamemodes/default.amx": "old default",
		"compiler/pawncc":       "#!/bin/sh\nfor arg do case \"$arg\" in -o*) printf 'new release' > \"${arg#-o}\";; esac; done\n",
		"pawn.json": `{"entry":"gamemodes/main.pwn","output":"gamemodes/default.amx","runtime_dir":"server",` +
			`"builds":[{"name":"release","compiler":{"path":"compiler"},"output":"build/release.amx"}]}`,
	} {
		filename := filepath.Join(project, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
		require.NoError(t, os.WriteFile(filename, []byte(data), 0o755))
	}
	pcx, err := NewPackageContext(NewPackageContextOptions{Parent: true, Dir: project, Platform: "linux", CacheDir: t.TempDir()})
	require.NoError(t, err)
	pcx.RuntimeEnv = &fakeRuntimeEnvironment{}
	pcx.BuildName = "release"

	return pcx
}

var _ RuntimeEnvironment = (*blockingRunEnvironment)(nil)
