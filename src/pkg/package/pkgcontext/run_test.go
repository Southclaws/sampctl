package pkgcontext

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type configuredGamemodeEnvironment struct {
	fakeRuntimeEnvironment
	generated run.Runtime
	executed  run.Runtime
}

func (env *configuredGamemodeEnvironment) GenerateConfig(cfg *run.Runtime) error {
	env.generated = *cfg
	return env.fakeRuntimeEnvironment.GenerateConfig(cfg)
}

func (env *configuredGamemodeEnvironment) Run(ctx context.Context, cfg run.Runtime, options runtimepkg.RunOptions) error {
	env.executed = cfg
	return env.fakeRuntimeEnvironment.Run(ctx, cfg, options)
}

func TestRunPrepareGamemodes(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		buildOutput   string
		runtime       *run.Runtime
		runtimes      []*run.Runtime
		runtimeName   string
		wantGamemodes []string
		wantScripts   []string
	}{
		{
			name:          "default gamemode from output",
			output:        "gamemodes/example.amx",
			wantGamemodes: []string{"example"},
		},
		{
			name:          "default gamemode from custom output directory",
			output:        "module/module.amx",
			wantGamemodes: []string{"module"},
		},
		{
			name:          "empty gamemodes use output",
			output:        "example.amx",
			runtime:       &run.Runtime{Gamemodes: []string{}},
			wantGamemodes: []string{"example"},
		},
		{
			name:   "filterscript keeps configured gamemode",
			output: "filterscripts/module.amx",
			runtime: &run.Runtime{
				Gamemodes:     []string{"test"},
				Filterscripts: []string{"module"},
			},
			wantGamemodes: []string{"test"},
			wantScripts:   []string{"module"},
		},
		{
			name:          "custom module keeps configured gamemode",
			output:        "module/module.amx",
			runtime:       &run.Runtime{Gamemodes: []string{"test"}},
			wantGamemodes: []string{"test"},
		},
		{
			name:          "configured gamemode rotation is preserved",
			output:        "module.amx",
			runtime:       &run.Runtime{Gamemodes: []string{"test", "other"}},
			wantGamemodes: []string{"test", "other"},
		},
		{
			name:   "named runtime keeps configured gamemode",
			output: "module/module.amx",
			runtimes: []*run.Runtime{
				{Name: "first", Gamemodes: []string{"first"}},
				{Name: "module", Gamemodes: []string{"test"}},
			},
			runtimeName:   "module",
			wantGamemodes: []string{"test"},
		},
		{
			name:   "filterscripts without gamemodes retain default behavior",
			output: "example.amx",
			runtime: &run.Runtime{
				Filterscripts: []string{"helper"},
			},
			wantGamemodes: []string{"example"},
			wantScripts:   []string{"helper"},
		},
		{
			name:          "default gamemode uses selected build output",
			output:        "gamemodes/default.amx",
			buildOutput:   "build/release.amx",
			wantGamemodes: []string{"release"},
		},
		{
			name:          "selected build keeps configured gamemode",
			output:        "gamemodes/default.amx",
			buildOutput:   "module/release.amx",
			runtime:       &run.Runtime{Gamemodes: []string{"test"}},
			wantGamemodes: []string{"test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			output := tt.output
			if tt.buildOutput != "" {
				output = tt.buildOutput
			}
			filename := filepath.Join(root, filepath.FromSlash(output))
			require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o700))
			require.NoError(t, os.WriteFile(filename, []byte{0}, 0o600))

			local := true
			env := &configuredGamemodeEnvironment{}
			pcx := newPackageContextBase(NewPackageContextOptions{
				Platform:   runtime.GOOS,
				CacheDir:   t.TempDir(),
				RuntimeEnv: env,
			})
			pcx.Package = pawnpackage.Package{
				Parent:    true,
				LocalPath: root,
				Output:    filepath.FromSlash(tt.output),
				Local:     &local,
				Format:    "json",
				Runtime:   tt.runtime,
				Runtimes:  tt.runtimes,
			}
			if tt.buildOutput != "" {
				pcx.Package.Build = &build.Config{Output: filepath.FromSlash(tt.buildOutput)}
			}
			pcx.Runtime = tt.runtimeName

			require.NoError(t, pcx.Run(t.Context(), io.Discard, nil))
			require.True(t, env.ensureCalled)
			require.True(t, env.generateCalled)
			require.True(t, env.runCalled)
			require.Equal(t, tt.wantGamemodes, env.generated.Gamemodes)
			require.Equal(t, tt.wantGamemodes, env.executed.Gamemodes)
			require.Equal(t, tt.wantScripts, env.executed.Filterscripts)
			require.Equal(t, filepath.FromSlash(tt.output), pcx.Package.Output)
			actualOutput, err := pcx.runtimeOutputPath()
			require.NoError(t, err)
			require.Equal(t, filename, actualOutput)
		})
	}
}

type configuredWatchEnvironment struct {
	fakeRuntimeEnvironment
	started chan run.Runtime
}

func (env *configuredWatchEnvironment) Run(ctx context.Context, cfg run.Runtime, _ runtimepkg.RunOptions) error {
	env.started <- cfg
	<-ctx.Done()
	return ctx.Err()
}

func TestRunWatchPreservesConfiguredGamemode(t *testing.T) {
	pcx := newRunBuildFixture(t)
	configPath := filepath.Join(pcx.Package.LocalPath, "pawn.json")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, json.Unmarshal(data, &config))
	config["runtime"] = map[string]any{"gamemodes": []string{"test"}}
	delete(config, "runtime_dir")
	data, err = json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(pcx.Package.LocalPath, "gamemodes", "test.amx"), []byte{0}, 0o600,
	))
	pcx.Package.Runtime = &run.Runtime{Gamemodes: []string{"test"}}
	pcx.Package.RuntimeDir = ""

	env := &configuredWatchEnvironment{started: make(chan run.Runtime, 1)}
	pcx.RuntimeEnv = env
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- pcx.RunWatch(ctx) }()

	select {
	case cfg := <-env.started:
		require.Equal(t, []string{"test"}, cfg.Gamemodes)
	case err := <-done:
		t.Fatalf("watcher exited before starting runtime: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("watcher did not start the configured gamemode")
	}
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("watcher ignored cancellation")
	}
}
