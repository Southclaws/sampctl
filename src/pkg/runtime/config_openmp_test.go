package runtime

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestOpenMPConfigDefaultsAndEmptyCollections(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "nil"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &run.Runtime{WorkingDir: t.TempDir(), RuntimeType: run.RuntimeTypeOpenMP}
			if empty {
				cfg.Plugins = []run.Plugin{}
				cfg.Components = []run.Plugin{}
				cfg.Gamemodes = []string{}
				cfg.Filterscripts = []string{}
				cfg.Extra = map[string]string{}
				cfg.Discord = map[string]any{}
			}

			require.NoError(t, GenerateConfig(cfg))
			content, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "config.json"))
			require.NoError(t, err)
			want := `{
				"announce": false, "enable_query": false, "use_dyn_ticks": true,
				"game": {"allow_interior_weapons": true, "use_entry_exit_markers": true,
					"use_nametag_los": true, "use_nametags": true, "use_stunt_bonuses": true,
					"validate_animations": true},
				"network": {"allow_037_clients": true},
				"logging": {"enable": true, "log_chat": true, "log_connection_messages": true,
					"log_deaths": true, "use_prefix": true, "use_timestamp": true},
				"rcon": {}, "pawn": {}
			}`
			var parsed map[string]any
			require.NoError(t, json.Unmarshal(content, &parsed))
			if empty {
				require.Equal(t, map[string]any{}, parsed["discord"])
				delete(parsed, "discord")
			}
			actual, err := json.Marshal(parsed)
			require.NoError(t, err)
			require.JSONEq(t, want, string(actual))
			require.NoFileExists(t, filepath.Join(cfg.WorkingDir, "server.cfg"))
		})
	}
}

func TestOpenMPConfigExplicitValuesAndNormalization(t *testing.T) {
	zero, disabled, empty := 0, false, ""
	language, website, mapName, mode := "-", "open.mp", "San Andreas", "Unknown"
	cfg := &run.Runtime{
		WorkingDir: t.TempDir(), RuntimeType: run.RuntimeTypeOpenMP,
		Hostname: &empty, MaxPlayers: &zero, MaxBots: &zero, Sleep: &zero,
		UseDynTicks: &disabled, Announce: &disabled, Query: &disabled,
		Port: &zero, Bind: &empty, Output: &disabled, RCON: &disabled,
		Language: &language, Weburl: &website, Mapname: &mapName, GamemodeText: &mode,
		Plugins:    []run.Plugin{"mysql.so", "streamer.dll", "keep.SO", "nested/plugin.so"},
		Components: []run.Plugin{"component.dll"},
		Gamemodes:  []string{"main.amx"}, Filterscripts: []string{"side.amx"},
	}
	before := *cfg

	require.NoError(t, GenerateConfig(cfg))
	require.Equal(t, before, *cfg)
	content, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "config.json"))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(content, &parsed))
	for _, key := range []string{"name", "max_players", "max_bots", "sleep", "language", "website"} {
		require.NotContains(t, parsed, key)
	}
	require.Equal(t, false, parsed["use_dyn_ticks"])
	require.Equal(t, false, parsed["announce"])
	require.Equal(t, false, parsed["enable_query"])
	require.Equal(t, map[string]any{"allow_037_clients": true, "port": float64(0), "bind": ""}, parsed["network"])
	require.Equal(t, false, parsed["logging"].(map[string]any)["enable"])
	require.Equal(t, map[string]any{"enable": false}, parsed["rcon"])
	require.NotContains(t, parsed["game"], "map")
	require.NotContains(t, parsed["game"], "mode")
	require.Equal(t, map[string]any{
		"legacy_plugins": []any{"mysql", "streamer", "keep.SO", "nested/plugin"},
		"components":     []any{"component"}, "main_scripts": []any{"main.amx"}, "side_scripts": []any{"side.amx"},
	}, parsed["pawn"])
	require.Equal(t, []run.Plugin{"mysql.so", "streamer.dll", "keep.SO", "nested/plugin.so"}, cfg.Plugins)
	require.Equal(t, []run.Plugin{"component.dll"}, cfg.Components)
}

func TestOpenMPConfigOverridePrecedenceAndInputOwnership(t *testing.T) {
	port := 7777
	nested := map[any]any{"enabled": false, "zero": 0, "empty": "", 42: "ignored"}
	cfg := &run.Runtime{
		WorkingDir: t.TempDir(), RuntimeType: run.RuntimeTypeOpenMP, Port: &port,
		Extra:   map[string]string{"name": "extra name", "network": "replaced", "announce": "legacy false"},
		Network: map[string]any{"port": 0, "custom": nested},
		Game:    map[string]any{"use_nametags": false, "custom": map[string]any{"value": nil}},
		Pawn:    map[string]any{"main_scripts": []string{}, "side_scripts": nil},
	}

	require.NoError(t, GenerateConfig(cfg))
	content, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "config.json"))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(content, &parsed))
	require.Equal(t, "extra name", parsed["name"])
	require.Equal(t, "legacy false", parsed["announce"])
	require.Equal(t, map[string]any{
		"port": float64(0), "custom": map[string]any{"enabled": false, "zero": float64(0), "empty": ""},
	}, parsed["network"])
	require.Equal(t, false, parsed["game"].(map[string]any)["use_nametags"])
	require.Equal(t, true, parsed["game"].(map[string]any)["validate_animations"])
	require.Equal(t, map[string]any{"value": nil}, parsed["game"].(map[string]any)["custom"])
	require.Equal(t, map[string]any{"main_scripts": []any{}, "side_scripts": nil}, parsed["pawn"])
	require.Equal(t, map[string]any{"port": 0, "custom": map[any]any{
		"enabled": false, "zero": 0, "empty": "", 42: "ignored",
	}}, cfg.Network)
	require.Equal(t, map[string]any{"use_nametags": false, "custom": map[string]any{"value": nil}}, cfg.Game)
	require.Equal(t, map[string]any{"main_scripts": []string{}, "side_scripts": nil}, cfg.Pawn)
	require.Equal(t, map[string]string{"name": "extra name", "network": "replaced", "announce": "legacy false"}, cfg.Extra)
	require.Equal(t, 7777, *cfg.Port)

	legacy, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "server.cfg"))
	require.NoError(t, err)
	require.Equal(t, "# server.cfg generated by sampctl for legacy plugin compatibility\nannounce legacy false\nname extra name\nnetwork replaced\n", string(legacy))
}

func TestOpenMPConfigFailureOrdering(t *testing.T) {
	for _, stage := range []string{"stale removal", "assembly", "create", "encode"} {
		t.Run(stage, func(t *testing.T) {
			cfg := &run.Runtime{
				WorkingDir: t.TempDir(), RuntimeType: run.RuntimeTypeOpenMP,
				Extra: map[string]string{"legacy": "value"},
			}
			legacyPath := filepath.Join(cfg.WorkingDir, "server.cfg")
			configPath := filepath.Join(cfg.WorkingDir, "config.json")
			require.NoError(t, os.WriteFile(configPath, []byte("original"), 0o600))
			nan := float32(math.NaN())
			switch stage {
			case "stale removal":
				require.NoError(t, os.Mkdir(legacyPath, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(legacyPath, "keep"), nil, 0o600))
				cfg.StreamDistance = &nan
			case "assembly":
				require.NoError(t, os.WriteFile(legacyPath, []byte("stale"), 0o600))
				cfg.StreamDistance = &nan
			case "create":
				require.NoError(t, os.Remove(configPath))
				require.NoError(t, os.Mkdir(configPath, 0o700))
				require.NoError(t, os.WriteFile(legacyPath, []byte("stale"), 0o600))
			case "encode":
				require.NoError(t, os.WriteFile(legacyPath, []byte("stale"), 0o600))
				cfg.Network = map[string]any{"invalid": make(chan int)}
			}

			err := GenerateConfig(cfg)
			require.Error(t, err)
			switch stage {
			case "stale removal":
				require.ErrorContains(t, err, "failed to remove stale server.cfg")
				require.DirExists(t, legacyPath)
			case "assembly":
				require.ErrorContains(t, err, "failed to marshal structured config")
				var unsupported *json.UnsupportedValueError
				require.ErrorAs(t, err, &unsupported)
			case "create":
				var pathErr *os.PathError
				require.ErrorAs(t, err, &pathErr)
				require.Equal(t, "open", pathErr.Op)
				require.Equal(t, configPath, pathErr.Path)
			case "encode":
				require.ErrorContains(t, err, "failed to write config.json")
				var unsupported *json.UnsupportedTypeError
				require.ErrorAs(t, err, &unsupported)
			}
			if stage != "stale removal" {
				require.NoDirExists(t, legacyPath)
				require.NoFileExists(t, legacyPath)
			}
			if stage != "create" {
				content, readErr := os.ReadFile(configPath)
				require.NoError(t, readErr)
				if stage == "encode" {
					require.Empty(t, content)
				} else {
					require.Equal(t, "original", string(content))
				}
			}
		})
	}
}
