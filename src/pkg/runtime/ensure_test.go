package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	infrafs "github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestPluginExtForFile(t *testing.T) {
	assert.Equal(t, ".dll", pluginExtForFile("windows"))
	assert.Equal(t, ".so", pluginExtForFile("linux"))
	assert.Equal(t, ".so", pluginExtForFile("darwin"))
	assert.Empty(t, pluginExtForFile("plan9"))
}

func TestEnsureStagedRuntimeReusesValidStage(t *testing.T) {
	rootDir := t.TempDir()
	cacheDir := filepath.Join(rootDir, "cache")
	platform := currentTestPlatform()
	seedRuntimeRemoteFixture(t, cacheDir, "0.3.7", platform)

	cfg := run.Runtime{
		WorkingDir: filepath.Join(rootDir, "server"),
		Platform:   platform,
		Format:     "json",
		Version:    "0.3.7",
		Mode:       run.Server,
	}

	manifest, stageDir, err := ensureStagedRuntime(context.Background(), cacheDir, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, manifest.Files)
	assert.FileExists(t, filepath.Join(stageDir, expectedRuntimeBinary(platform)))

	require.NoError(t, os.Remove(filepath.Join(cacheDir, "runtimes.json")))

	manifestAgain, stageDirAgain, err := ensureStagedRuntime(context.Background(), cacheDir, cfg)
	require.NoError(t, err)
	assert.Equal(t, stageDir, stageDirAgain)
	assert.True(t, manifestsEqual(manifest, manifestAgain))
	assert.FileExists(t, filepath.Join(stageDirAgain, expectedRuntimeBinary(platform)))
	assert.NoFileExists(t, filepath.Join(cacheDir, "runtimes.json"))
	assert.NoDirExists(t, cfg.WorkingDir)
}

func TestEnsureBinariesContextInstallsAndReusesRuntime(t *testing.T) {
	rootDir := t.TempDir()
	cacheDir := filepath.Join(rootDir, "cache")
	workingDir := filepath.Join(rootDir, "server")
	platform := currentTestPlatform()
	seedRuntimeRemoteFixture(t, cacheDir, "0.3.7", platform)

	cfg := run.Runtime{
		WorkingDir: workingDir,
		Platform:   platform,
		Format:     "json",
		Version:    "0.3.7",
		Mode:       run.Server,
	}

	info, err := EnsureBinariesContext(context.Background(), cacheDir, cfg)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, cfg.Version, info.Version)
	assert.Equal(t, cfg.Platform, info.Platform)
	assert.FileExists(t, filepath.Join(workingDir, expectedRuntimeBinary(platform)))

	require.NoError(t, os.Remove(filepath.Join(cacheDir, "runtimes.json")))

	infoAgain, err := EnsureBinariesContext(context.Background(), cacheDir, cfg)
	require.NoError(t, err)
	require.NotNil(t, infoAgain)
	assert.Equal(t, info.Files, infoAgain.Files)
	assert.FileExists(t, filepath.Join(workingDir, expectedRuntimeBinary(platform)))
	assert.NoFileExists(t, filepath.Join(cacheDir, "runtimes.json"))
	assert.NoFileExists(t, filepath.Join(workingDir, runtimeManifestFileName))
}

func TestEnsureBinariesWrapper(t *testing.T) {
	rootDir := t.TempDir()
	cacheDir := filepath.Join(rootDir, "cache")
	workingDir := filepath.Join(rootDir, "server")
	platform := currentTestPlatform()
	seedRuntimeRemoteFixture(t, cacheDir, "0.3.7", platform)

	info, err := EnsureBinaries(cacheDir, run.Runtime{
		WorkingDir: workingDir,
		Platform:   platform,
		Format:     "json",
		Version:    "0.3.7",
		Mode:       run.Server,
	})
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.FileExists(t, filepath.Join(workingDir, expectedRuntimeBinary(platform)))
}

func TestEnsureBinariesContextReplacesMismatchedInstalledRuntime(t *testing.T) {
	rootDir := t.TempDir()
	cacheDir := filepath.Join(rootDir, "cache")
	workingDir := filepath.Join(rootDir, "server")
	platform := currentTestPlatform()
	seedRuntimeRemoteFixture(t, cacheDir, "0.3.7", platform)

	require.NoError(t, os.MkdirAll(workingDir, 0o755))
	oldBinary := filepath.Join(workingDir, expectedRuntimeBinary(platform))
	require.NoError(t, os.WriteFile(oldBinary, []byte("stale"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "obsolete-file"), []byte("old"), 0o644))

	oldManifest := runtimeManifest{
		Version:     "old-version",
		Platform:    platform,
		RuntimeType: run.RuntimeTypeSAMP,
		Files: []runtimeFileInfo{
			{Path: filepath.Base(oldBinary), Size: int64(len("stale")), Hash: mustHashFile(t, oldBinary), Mode: 0o755},
			{Path: "obsolete-file", Size: int64(len("old")), Hash: mustHashFile(t, filepath.Join(workingDir, "obsolete-file")), Mode: 0o644},
		},
	}

	lf := lockfile.New("1.2.3")
	files := make([]lockfile.LockedFileInfo, len(oldManifest.Files))
	for i, file := range oldManifest.Files {
		files[i] = lockfile.LockedFileInfo(file)
	}
	lf.SetRuntime(oldManifest.Version, oldManifest.Platform, string(oldManifest.RuntimeType), files)
	require.NoError(t, lockfile.Save(workingDir, lf))

	cfg := run.Runtime{
		WorkingDir: workingDir,
		Platform:   platform,
		Format:     "json",
		Version:    "0.3.7",
		Mode:       run.Server,
	}

	info, err := EnsureBinariesContext(context.Background(), cacheDir, cfg)
	require.NoError(t, err)
	require.NotNil(t, info)

	contents, err := os.ReadFile(oldBinary)
	require.NoError(t, err)
	assert.Equal(t, "fixture", string(contents))
	assert.NoFileExists(t, filepath.Join(workingDir, "obsolete-file"))
}

func TestEnsureRejectsInvalidConfig(t *testing.T) {
	err := Ensure(context.Background(), nil, &run.Runtime{}, false)
	require.EqualError(t, err, "WorkingDir empty")
}

func TestEnsureUsesConfigDirAndSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)

	cacheDir, err := infrafs.ConfigDir()
	require.NoError(t, err)

	platform := currentTestPlatform()
	seedRuntimeRemoteFixture(t, cacheDir, "0.3.7", platform)

	workingDir := filepath.Join(t.TempDir(), "server")
	cfg := run.Runtime{
		WorkingDir: workingDir,
		Platform:   platform,
		Format:     "json",
		Version:    "0.3.7",
		Mode:       run.Server,
	}

	err = Ensure(context.Background(), nil, &cfg, false)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(workingDir, expectedRuntimeBinary(platform)))
	assert.DirExists(t, filepath.Join(workingDir, getPluginDirectory()))
}

func mustHashFile(t *testing.T, path string) string {
	t.Helper()

	hash, _, err := hashFile(path)
	require.NoError(t, err)
	return hash
}
