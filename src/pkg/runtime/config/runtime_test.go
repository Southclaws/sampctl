package run

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFromDirPrefersJSON(t *testing.T) {
	t.Parallel()

	basic := basicRuntime()
	full := fullRuntime()
	full.Plugins = []Plugin{"streamer", "AmyrAhmady/samp-plugin-crashdetect"}

	for _, tc := range []struct {
		name string
		dir  string
		want Runtime
	}{
		{name: "minimal config", dir: "./tests/load-both", want: basic},
		{name: "config with plugins", dir: "./tests/load-yaml", want: full},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			want.Format = "json"
			want.WorkingDir = tc.dir

			got, err := RuntimeFromDir(tc.dir)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestRuntimeFromJSON(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path string
		want Runtime
	}{
		{name: "minimal config", path: "tests/load-both/samp.json", want: basicRuntime()},
		{name: "server settings", path: "tests/load-json/samp.json", want: fullRuntime()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			want.Format = "json"

			got, err := RuntimeFromJSON(tc.path)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestRuntimeFromYAML(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path string
		want Runtime
	}{
		{name: "minimal config", path: "tests/load-both/samp.yaml", want: basicRuntime()},
		{name: "server settings", path: "tests/load-yaml/samp.yaml", want: fullRuntime()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			want.Format = "yaml"

			got, err := RuntimeFromYAML(tc.path)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestRuntimeToJSON(t *testing.T) {
	t.Parallel()

	cfg := fullRuntime()
	cfg.WorkingDir = t.TempDir()
	want, err := os.ReadFile("tests/generate-json/samp.json")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.WorkingDir, "samp.json"), bytes.Repeat([]byte("x"), len(want)+1), 0o600))

	err = cfg.ToJSON()

	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "samp.json"))
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestRuntimeToYAML(t *testing.T) {
	t.Parallel()

	cfg := fullRuntime()
	cfg.WorkingDir = t.TempDir()
	want, err := os.ReadFile("tests/generate-yaml/samp.yaml")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.WorkingDir, "samp.yaml"), bytes.Repeat([]byte("x"), len(want)+1), 0o600))

	err = cfg.ToYAML()

	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "samp.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func basicRuntime() Runtime {
	return Runtime{
		Gamemodes:    []string{"rivershell"},
		RCONPassword: new("hello"),
	}
}

func fullRuntime() Runtime {
	return Runtime{
		Gamemodes:    []string{"rivershell", "baserace"},
		RCONPassword: new("test"),
		Port:         new(8080),
		Hostname:     new("Test"),
		MaxPlayers:   new(32),
		Language:     new("English"),
		Announce:     new(true),
		RCON:         new(true),
	}
}
