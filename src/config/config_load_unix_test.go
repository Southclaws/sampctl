//go:build linux || darwin

package config

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigRejectsFIFO(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	path := filepath.Join(cacheDir, "config.json")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	_, err := LoadConfig(cacheDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}
