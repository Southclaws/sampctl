package run

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestRuntimeValidate(t *testing.T) {
	base := Runtime{WorkingDir: ".", Platform: "linux", Format: "json", Version: "0.3.7", Mode: Server}
	require.NoError(t, base.Validate())

	missingWorking := base
	missingWorking.WorkingDir = ""
	require.ErrorContains(t, missingWorking.Validate(), "WorkingDir empty")

	missingPlatform := base
	missingPlatform.Platform = ""
	require.ErrorContains(t, missingPlatform.Validate(), "Platform empty")

	missingFormat := base
	missingFormat.Format = ""
	require.ErrorContains(t, missingFormat.Validate(), "Format empty")

	missingVersion := base
	missingVersion.Version = ""
	require.ErrorContains(t, missingVersion.Validate(), "Version empty")

	missingMode := base
	missingMode.Mode = ""
	require.ErrorContains(t, missingMode.Validate(), "Mode empty")
}

func TestRuntimeFromDirMissingConfig(t *testing.T) {
	_, err := RuntimeFromDir(t.TempDir())
	require.ErrorContains(t, err, "does not contain a samp.json or samp.yaml")
}

func TestRuntimeFromJSONAndYAMLErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	_, err := RuntimeFromJSON(missing)
	require.Error(t, err)

	invalidJSON := filepath.Join(t.TempDir(), "invalid.json")
	require.NoError(t, os.WriteFile(invalidJSON, []byte(`{"gamemodes":`), 0o644))
	_, err = RuntimeFromJSON(invalidJSON)
	require.ErrorContains(t, err, "failed to unmarshal samp.json")

	invalidYAML := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidYAML, []byte("gamemodes: ["), 0o644))
	_, err = RuntimeFromYAML(invalidYAML)
	require.ErrorContains(t, err, "failed to unmarshal samp.yaml")
}

func TestResolveRemotePlugins(t *testing.T) {
	rt := &Runtime{Plugins: []Plugin{"streamer", "AmyrAhmady/samp-plugin-crashdetect:v4.22", "plugin://local/plugins/test"}}
	rt.ResolveRemotePlugins()

	assert.Equal(t, []Plugin{"streamer"}, rt.Plugins)
	require.Len(t, rt.PluginDeps, 2)
	assert.Equal(t, versioning.DependencyMeta{Site: "github.com", User: "AmyrAhmady", Repo: "samp-plugin-crashdetect", Tag: "v4.22"}, rt.PluginDeps[0])
	assert.Equal(t, "plugin", rt.PluginDeps[1].Scheme)
	assert.Equal(t, "plugins/test", rt.PluginDeps[1].Local)

	var nilRT *Runtime
	nilRT.ResolveRemotePlugins()
}

func TestRuntimeDefaultsHelpers(t *testing.T) {
	defaults := GetRuntimeDefaultValues()
	assert.Equal(t, 8192, defaults.Port)
	assert.Equal(t, "", defaults.RCONPassword)
	assert.Equal(t, "SA-MP Server", defaults.Hostname)
	assert.Equal(t, 50, defaults.MaxPlayers)
	assert.Equal(t, "-", defaults.Language)

	rt := GetRuntimeDefault()
	require.NotNil(t, rt)
	assert.Equal(t, "0.3.7", rt.Version)
	assert.Equal(t, 7777, *rt.Port)
	assert.Equal(t, "password", *rt.RCONPassword)
	assert.Equal(t, Server, rt.Mode)
}

func TestApplyRuntimeDefaults(t *testing.T) {
	rt := &Runtime{}
	ApplyRuntimeDefaults(rt)
	assert.Equal(t, "0.3.7", rt.Version)
	assert.Equal(t, runtime.GOOS, rt.Platform)
	require.NotNil(t, rt.Port)
	assert.Equal(t, 7777, *rt.Port)
	require.NotNil(t, rt.RCONPassword)
	assert.Equal(t, "password", *rt.RCONPassword)
	assert.Equal(t, Server, rt.Mode)

	customPort := 9000
	customPassword := "secret"
	custom := &Runtime{Version: "v1", Platform: "windows", Port: &customPort, RCONPassword: &customPassword, Mode: MainOnly}
	ApplyRuntimeDefaults(custom)
	assert.Equal(t, "v1", custom.Version)
	assert.Equal(t, "windows", custom.Platform)
	assert.Equal(t, 9000, *custom.Port)
	assert.Equal(t, "secret", *custom.RCONPassword)
	assert.Equal(t, MainOnly, custom.Mode)

	assert.NotPanics(t, func() { ApplyRuntimeDefaults(nil) })
}

func TestPluginAsDep(t *testing.T) {
	dep, err := Plugin("Southclaws/pawn-errors:1.2.3").AsDep()
	require.NoError(t, err)
	assert.Equal(t, versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-errors", Tag: "1.2.3"}, dep)

	_, err = Plugin("streamer").AsDep()
	require.Error(t, err)
}

func TestRuntimeToFile(t *testing.T) {
	jsonDir := t.TempDir()
	jsonCfg := Runtime{WorkingDir: jsonDir, Format: "json", Gamemodes: []string{"gm"}}
	require.NoError(t, jsonCfg.ToFile())
	assert.FileExists(t, filepath.Join(jsonDir, "samp.json"))

	yamlDir := t.TempDir()
	yamlCfg := Runtime{WorkingDir: yamlDir, Format: "yaml", Gamemodes: []string{"gm"}}
	require.NoError(t, yamlCfg.ToFile())
	assert.FileExists(t, filepath.Join(yamlDir, "samp.yaml"))

	err := (Runtime{WorkingDir: t.TempDir(), Format: "toml"}).ToFile()
	require.ErrorContains(t, err, "no format associated")
}

func TestCloneWithoutDefaults(t *testing.T) {
	rootLink := false
	port := 9000
	rconPassword := "secret"
	hostname := "Custom"
	maxPlayers := 75
	language := "English"
	connectCookies := false
	maxBots := 10
	mode := MainOnly
	runtimeType := RuntimeTypeOpenMP
	mapname := "San Andreas"
	game := map[string]any{"weather": "sunny"}
	extra := map[string]string{"feature": "enabled"}

	rt := &Runtime{
		WorkingDir:     "/tmp/project",
		Platform:       "linux",
		Format:         "json",
		Version:        "v1.0.0-openmp",
		Mode:           mode,
		RuntimeType:    runtimeType,
		RootLink:       rootLink,
		Gamemodes:      []string{"gm"},
		Filterscripts:  []string{"fs"},
		Plugins:        []Plugin{"plugin"},
		Components:     []Plugin{"Pawn"},
		RCONPassword:   &rconPassword,
		Port:           &port,
		Hostname:       &hostname,
		MaxPlayers:     &maxPlayers,
		Language:       &language,
		Mapname:        &mapname,
		ConnectCookies: &connectCookies,
		Extra:          extra,
		MaxBots:        &maxBots,
		Game:           game,
	}

	cloned := CloneWithoutDefaults(rt)
	require.NotNil(t, cloned)
	assert.Equal(t, rt.WorkingDir, cloned.WorkingDir)
	assert.Equal(t, mode, cloned.Mode)
	assert.Equal(t, runtimeType, cloned.RuntimeType)
	assert.Equal(t, rootLink, cloned.RootLink)
	assert.Equal(t, port, *cloned.Port)
	assert.Equal(t, rconPassword, *cloned.RCONPassword)
	assert.Equal(t, hostname, *cloned.Hostname)
	assert.Equal(t, maxPlayers, *cloned.MaxPlayers)
	assert.Equal(t, language, *cloned.Language)
	assert.Equal(t, mapname, *cloned.Mapname)
	assert.Equal(t, rt.Components, cloned.Components)
	require.NotNil(t, cloned.ConnectCookies)
	assert.Equal(t, connectCookies, *cloned.ConnectCookies)
	assert.Equal(t, extra, cloned.Extra)
	require.NotNil(t, cloned.MaxBots)
	assert.Equal(t, maxBots, *cloned.MaxBots)
	assert.Equal(t, game, cloned.Game)

	defaultLanguage := "-"
	defaultPort := 8192
	defaultHost := "SA-MP Server"
	defaultPlayers := 50
	rt2 := &Runtime{Language: &defaultLanguage, Port: &defaultPort, Hostname: &defaultHost, MaxPlayers: &defaultPlayers}
	assert.Nil(t, CloneWithoutDefaults(rt2))

	assert.Nil(t, CloneWithoutDefaults(&Runtime{}))
	assert.Nil(t, CloneWithoutDefaults(&Runtime{
		WorkingDir: "/tmp/project",
		Platform:   "linux",
		Format:     "json",
	}))

	assert.Nil(t, CloneWithoutDefaults(nil))
}
