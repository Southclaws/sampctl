package versioning

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadLocalDependencyOverrides(t *testing.T) {
	cacheDir := t.TempDir()
	cachedPath := filepath.Join(cacheDir, "remote-dependency-overrides.json")
	localPath := filepath.Join(cacheDir, "dependency-overrides.json")
	require.NoError(t, os.WriteFile(cachedPath, []byte(`{"overrides":{"old/repo":"cached/repo","cached/only":"new/only"}}`), 0o644))
	require.NoError(t, os.WriteFile(localPath, []byte(`{"overrides":{"old/repo":"local/repo"}}`), 0o644))
	oldTime := time.Now().Add(-2 * CacheValidityDuration)
	require.NoError(t, os.Chtimes(cachedPath, oldTime, oldTime))
	originalLoader := remoteOverridesLoader
	remoteOverridesLoader = func(context.Context) map[string]string {
		t.Fatal("local diagnostics fetched remote overrides")
		return nil
	}
	t.Cleanup(func() { remoteOverridesLoader = originalLoader })

	overrides, err := LoadLocalDependencyOverrides(cacheDir)
	require.NoError(t, err)
	assert.Equal(t, "local/repo", overrides["old/repo"])
	assert.Equal(t, "new/only", overrides["cached/only"])
	assert.Equal(t, DependencyOverrides["Zeex/samp-plugin-crashdetect"], overrides["Zeex/samp-plugin-crashdetect"])
	info, err := os.Stat(cachedPath)
	require.NoError(t, err)
	assert.WithinDuration(t, oldTime, info.ModTime(), time.Second)
}

func TestLoadLocalDependencyOverridesMissingAndInvalid(t *testing.T) {
	t.Run("missing files remain absent", func(t *testing.T) {
		cacheDir := filepath.Join(t.TempDir(), "missing")
		overrides, err := LoadLocalDependencyOverrides(cacheDir)
		require.NoError(t, err)
		assert.Equal(t, DependencyOverrides, overrides)
		_, err = os.Stat(cacheDir)
		assert.True(t, os.IsNotExist(err))
	})
	t.Run("malformed files preserve available overrides and errors", func(t *testing.T) {
		cacheDir := t.TempDir()
		path := filepath.Join(cacheDir, "dependency-overrides.json")
		require.NoError(t, os.WriteFile(path, []byte("{"), 0o644))
		overrides, err := LoadLocalDependencyOverrides(cacheDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), path)
		assert.Equal(t, DependencyOverrides, overrides)
	})
}

func TestExplodeWithOverridesIsLocal(t *testing.T) {
	originalLoader := remoteOverridesLoader
	remoteOverridesLoader = func(context.Context) map[string]string {
		t.Fatal("local parsing fetched remote overrides")
		return nil
	}
	ResetDependencyOverrides()
	t.Cleanup(func() {
		remoteOverridesLoader = originalLoader
		ResetDependencyOverrides()
	})

	tests := []struct {
		input     DependencyString
		overrides map[string]string
		want      DependencyMeta
		wantErr   bool
	}{
		{input: "old/repo:1.2", overrides: map[string]string{"old/repo": "new/repo"}, want: DependencyMeta{Site: "github.com", User: "new", Repo: "repo", Tag: "1.2"}},
		{input: "old/repo:1.2", overrides: map[string]string{"old/repo": "new/repo:2.0"}, want: DependencyMeta{Site: "github.com", User: "new", Repo: "repo", Tag: "2.0"}},
		{input: "includes://local/../includes", want: DependencyMeta{Scheme: "includes", Local: "../includes"}},
		{input: "plugin://owner/repo@main", want: DependencyMeta{Scheme: "plugin", Site: "github.com", User: "owner", Repo: "repo", Branch: "main"}},
		{input: "includes://local/", wantErr: true},
		{input: "not-a-dependency", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got, err := tt.input.ExplodeWithOverrides(tt.overrides)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	assert.Nil(t, loadedOverrides)
}
