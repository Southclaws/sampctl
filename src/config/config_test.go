package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindConfigFilePrefersJSON(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	jsonPath := filepath.Join(cacheDir, "config.json")
	yamlPath := filepath.Join(cacheDir, "config.yaml")

	require.NoError(t, os.WriteFile(jsonPath, []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(yamlPath, []byte("default_user: bob\n"), 0o600))

	path, ok := findConfigFile(cacheDir)
	require.True(t, ok)
	assert.Equal(t, jsonPath, path)
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := defaultConfig("alice")

	require.NotNil(t, cfg.HideVersionUpdateMessage)
	assert.Equal(t, "alice", cfg.DefaultUser)
	assert.False(t, *cfg.HideVersionUpdateMessage)
}

func TestLoadOrCreateConfigCreatesDefaultConfig(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	oldLookup := lookupCurrentUsername
	lookupCurrentUsername = func() string { return "fixture-user" }
	t.Cleanup(func() {
		lookupCurrentUsername = oldLookup
	})

	cfg, err := LoadOrCreateConfig(cacheDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.HideVersionUpdateMessage)
	assert.Equal(t, "fixture-user", cfg.DefaultUser)
	assert.False(t, *cfg.HideVersionUpdateMessage)

	contents, err := os.ReadFile(filepath.Join(cacheDir, "config.json"))
	require.NoError(t, err)

	var written Config
	require.NoError(t, json.Unmarshal(contents, &written))
	assert.Equal(t, "fixture-user", written.DefaultUser)
	require.NotNil(t, written.HideVersionUpdateMessage)
	assert.False(t, *written.HideVersionUpdateMessage)
}

func TestLoadOrCreateConfigLoadsExistingAndNormalizesHideFlag(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	contents := []byte(`{"default_user":"alice"}`)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "config.json"), contents, 0o600))

	cfg, err := LoadOrCreateConfig(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, "alice", cfg.DefaultUser)
	require.NotNil(t, cfg.HideVersionUpdateMessage)
	assert.False(t, *cfg.HideVersionUpdateMessage)
}

func TestWriteConfigWritesJSONFile(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	hideUpdates := true
	input := Config{
		DefaultUser:              "alice",
		GitHubToken:              "token",
		HideVersionUpdateMessage: &hideUpdates,
	}

	require.NoError(t, WriteConfig(cacheDir, input))

	contents, err := os.ReadFile(filepath.Join(cacheDir, "config.json"))
	require.NoError(t, err)

	var got Config
	require.NoError(t, json.Unmarshal(contents, &got))
	assert.Equal(t, input.DefaultUser, got.DefaultUser)
	assert.Equal(t, input.GitHubToken, got.GitHubToken)
	require.NotNil(t, got.HideVersionUpdateMessage)
	assert.True(t, *got.HideVersionUpdateMessage)
}
