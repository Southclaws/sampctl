package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareRuntimeDirectory(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	scriptfiles := filepath.Join(t.TempDir(), "scriptfiles")
	require.NoError(t, os.MkdirAll(scriptfiles, 0o755))

	platform := currentTestPlatform()
	expectedBinary := seedRuntimeCacheFixture(t, cacheDir, "0.3.7", platform)

	err := PrepareRuntimeDirectory(cacheDir, "0.3.7", platform, scriptfiles)
	require.NoError(t, err)

	runtimeDir := GetRuntimePath(cacheDir, "0.3.7")
	assert.FileExists(t, filepath.Join(runtimeDir, expectedBinary))
	assert.DirExists(t, filepath.Join(runtimeDir, "gamemodes"))
	assert.DirExists(t, filepath.Join(runtimeDir, "filterscripts"))
	assert.DirExists(t, filepath.Join(runtimeDir, "plugins"))

	linkTarget, err := os.Readlink(filepath.Join(runtimeDir, "scriptfiles"))
	require.NoError(t, err)
	assert.Equal(t, scriptfiles, linkTarget)
}

func TestCopyFileToRuntime(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	version := "0.3.7"
	runtimeDir := GetRuntimePath(cacheDir, version)
	require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "gamemodes"), 0o755))

	amxPath := filepath.Join(t.TempDir(), "test.amx")
	require.NoError(t, os.WriteFile(amxPath, []byte("amx"), 0o644))

	require.NoError(t, CopyFileToRuntime(cacheDir, version, amxPath))
	assert.FileExists(t, filepath.Join(runtimeDir, "gamemodes", "test.amx"))

	txtPath := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(txtPath, []byte("txt"), 0o644))
	err := CopyFileToRuntime(cacheDir, version, txtPath)
	require.Error(t, err)
}

func TestGetRuntimePath(t *testing.T) {
	t.Parallel()

	assert.Equal(t, filepath.Join("cache", "runtime", "0.3.7"), GetRuntimePath("cache", "0.3.7"))
}
