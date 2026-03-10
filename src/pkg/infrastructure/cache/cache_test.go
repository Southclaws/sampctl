package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infrafs "github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestIsFresh(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cache.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"ok":true}`), 0o644))

	assert.False(t, IsFresh(path, 0))
	assert.True(t, IsFresh(path, time.Hour))

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(path, stale, stale))
	assert.False(t, IsFresh(path, time.Minute))
	assert.False(t, IsFresh(filepath.Join(t.TempDir(), "missing.json"), time.Hour))
}

func TestReadJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"test","count":2}`), 0o644))

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	value, err := ReadJSON[payload](path)
	require.NoError(t, err)
	assert.Equal(t, payload{Name: "test", Count: 2}, value)

	_, err = ReadJSON[payload](filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)

	invalidPath := filepath.Join(t.TempDir(), "invalid.json")
	require.NoError(t, os.WriteFile(invalidPath, []byte(`{"name":`), 0o644))
	_, err = ReadJSON[payload](invalidPath)
	require.Error(t, err)
}

func TestGetOrRefreshJSONReadsFreshCache(t *testing.T) {
	t.Parallel()

	type payload struct {
		Value string `json:"value"`
	}

	path := filepath.Join(t.TempDir(), "fresh.json")
	require.NoError(t, infrafs.WriteJSONAtomic(path, payload{Value: "cached"}, 0o755, 0o644))

	called := false
	value, refreshed, err := GetOrRefreshJSON(context.Background(), path, time.Hour, 0o755, 0o644, func(context.Context) (payload, error) {
		called = true
		return payload{Value: "fetched"}, nil
	})
	require.NoError(t, err)
	assert.False(t, refreshed)
	assert.False(t, called)
	assert.Equal(t, payload{Value: "cached"}, value)
}

func TestGetOrRefreshJSONFetchesAndWrites(t *testing.T) {
	t.Parallel()

	type payload struct {
		Value string `json:"value"`
	}

	path := filepath.Join(t.TempDir(), "stale.json")
	value, refreshed, err := GetOrRefreshJSON(context.Background(), path, time.Hour, 0o755, 0o644, func(context.Context) (payload, error) {
		return payload{Value: "fetched"}, nil
	})
	require.NoError(t, err)
	assert.True(t, refreshed)
	assert.Equal(t, payload{Value: "fetched"}, value)

	stored, err := ReadJSON[payload](path)
	require.NoError(t, err)
	assert.Equal(t, payload{Value: "fetched"}, stored)
}

func TestGetOrRefreshJSONFetchError(t *testing.T) {
	t.Parallel()

	type payload struct {
		Value string `json:"value"`
	}

	path := filepath.Join(t.TempDir(), "failed.json")
	_, refreshed, err := GetOrRefreshJSON[payload](context.Background(), path, time.Hour, 0o755, 0o644, func(context.Context) (payload, error) {
		return payload{}, assert.AnError
	})
	require.Error(t, err)
	assert.False(t, refreshed)
	assert.NoFileExists(t, path)
}
