package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigDoesNotCreateDefaults(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "missing")
	cfg, err := LoadConfig(cacheDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.HideVersionUpdateMessage)
	assert.False(t, *cfg.HideVersionUpdateMessage)
	assert.NoDirExists(t, cacheDir)
}

func TestLoadConfigPreservesExistingFiles(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"config.json", "config.yaml"} {
		t.Run(name, func(t *testing.T) {
			cacheDir := t.TempDir()
			contents := []byte(`{"default_user":"fixture-user"}`)
			if name == "config.yaml" {
				contents = []byte("defaultuser: fixture-user\n")
			}
			path := filepath.Join(cacheDir, name)
			require.NoError(t, os.WriteFile(path, contents, 0o600))
			before, err := os.Stat(path)
			require.NoError(t, err)

			cfg, err := LoadConfig(cacheDir)
			require.NoError(t, err)
			assert.Equal(t, "fixture-user", cfg.DefaultUser)

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, contents, got)

			after, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, before.ModTime(), after.ModTime())
		})
	}
}

func TestLoadConfigPreservesParseAndAccessErrors(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "config.json"), []byte("{broken"), 0o600))

	_, err := LoadConfig(cacheDir)
	var syntaxErr *json.SyntaxError
	assert.True(t, errors.As(err, &syntaxErr), "LoadConfig must preserve malformed JSON error")

	cacheFile := filepath.Join(t.TempDir(), "cache-file")
	require.NoError(t, os.WriteFile(cacheFile, []byte("file"), 0o600))

	_, err = LoadConfig(cacheFile)
	var pathErr *os.PathError
	assert.True(t, errors.As(err, &pathErr), "LoadConfig must preserve inaccessible cache error")
}

func TestLoadConfigRejectsNonRegularFiles(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	path := filepath.Join(cacheDir, "config.json")
	require.NoError(t, os.Mkdir(path, 0o700))

	_, err := LoadConfig(cacheDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
	assert.DirExists(t, path)
}

func TestLoadConfigRejectsDanglingSymlink(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	path := filepath.Join(cacheDir, "config.json")
	if err := os.Symlink(filepath.Join(cacheDir, "missing.json"), path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := LoadConfig(cacheDir)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), path)

	_, err = os.Lstat(path)
	require.NoError(t, err)
}

func TestLoadConfigAppliesEnvironmentWithoutFile(t *testing.T) {
	t.Setenv("SAMPCTL_DEFAULT_USER", "env-user")
	t.Setenv("SAMPCTL_HIDE_VERSION_UPDATE_MESSAGE", "true")

	cacheDir := filepath.Join(t.TempDir(), "missing")
	cfg, err := LoadConfig(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, "env-user", cfg.DefaultUser)
	require.NotNil(t, cfg.HideVersionUpdateMessage)
	assert.True(t, *cfg.HideVersionUpdateMessage)
	assert.NoDirExists(t, cacheDir)
}

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

func TestLoadOrCreateConfigAppliesEnvironmentOverrides(t *testing.T) {
	t.Setenv("SAMPCTL_DEFAULT_USER", "env-user")
	t.Setenv("SAMPCTL_GITHUB_TOKEN", "env-token")
	t.Setenv("SAMPCTL_GIT_USERNAME", "env-git-user")
	t.Setenv("SAMPCTL_GIT_PASSWORD", "env-git-password")
	t.Setenv("SAMPCTL_HIDE_VERSION_UPDATE_MESSAGE", "true")
	t.Setenv("CI", "1")

	cacheDir := t.TempDir()
	contents := []byte(`{"default_user":"file-user","hide_version_update_message":false}`)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "config.json"), contents, 0o600))

	cfg, err := LoadOrCreateConfig(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, "env-user", cfg.DefaultUser)
	assert.Equal(t, "env-token", cfg.GitHubToken)
	assert.Equal(t, "env-git-user", cfg.GitUsername)
	assert.Equal(t, "env-git-password", cfg.GitPassword)
	require.NotNil(t, cfg.HideVersionUpdateMessage)
	assert.True(t, *cfg.HideVersionUpdateMessage)
	assert.Equal(t, "1", cfg.CI)
}
