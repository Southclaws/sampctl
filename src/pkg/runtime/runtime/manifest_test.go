package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

func TestRuntimeManifestLifecycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := run.Runtime{Version: "0.3.7", Platform: "linux"}

	require.NoError(t, os.MkdirAll(filepath.Join(root, "plugins"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugins", "mysql.so"), []byte("plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "server.cfg"), []byte("echo test"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, runtimeManifestDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, runtimeManifestRelativePath), []byte("ignore me"), 0o600))

	manifest, err := buildRuntimeManifest(root, cfg)
	require.NoError(t, err)
	assert.True(t, manifest.matchesRuntime(cfg))
	require.Len(t, manifest.Files, 2)
	assert.Equal(t, "plugins/mysql.so", manifest.Files[0].Path)
	assert.Equal(t, "server.cfg", manifest.Files[1].Path)

	manifestPath := runtimeManifestPath(root)
	require.NoError(t, writeRuntimeManifest(manifestPath, manifest))
	loaded, err := readRuntimeManifest(manifestPath)
	require.NoError(t, err)
	assert.True(t, manifestsEqual(manifest, loaded))
	require.NoError(t, verifyRuntimeManifest(loaded, root))

	dest := t.TempDir()
	require.NoError(t, copyRuntimeFiles(loaded, root, dest))
	assert.FileExists(t, filepath.Join(dest, "plugins", "mysql.so"))
	assert.FileExists(t, filepath.Join(dest, "server.cfg"))

	info, err := GetRuntimeManifestInfo(root)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, cfg.Version, info.Version)
	assert.Equal(t, cfg.Platform, info.Platform)
	require.Len(t, info.Files, 2)

	require.NoError(t, removeRuntimeFiles(loaded, dest))
	assert.NoFileExists(t, filepath.Join(dest, "plugins", "mysql.so"))
	assert.NoFileExists(t, filepath.Join(dest, "server.cfg"))
}

func TestRuntimeManifestHelpers(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("payload"), 0o644))

	hash, size, err := hashFile(path)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.EqualValues(t, len("payload"), size)

	cfg := run.Runtime{Version: "v1.0.0-openmp", Platform: "linux", RuntimeType: run.RuntimeTypeOpenMP}
	manifest := runtimeManifest{Version: "v1.0.0-openmp", Platform: "linux", RuntimeType: run.RuntimeTypeOpenMP}
	assert.True(t, manifest.matchesRuntime(cfg))
	assert.False(t, manifest.matchesRuntime(run.Runtime{Version: "0.3.7", Platform: "linux"}))

	assert.Equal(t, filepath.Join(path, runtimeManifestRelativePath), runtimeManifestPath(path))

	info, err := GetRuntimeManifestInfo(t.TempDir())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestVerifyRuntimeManifestDetectsMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "server.cfg"), []byte("current"), 0o644))

	manifest := runtimeManifest{
		Files: []runtimeFileInfo{{
			Path: "server.cfg",
			Size: int64(len("other")),
			Hash: "deadbeef",
		}},
	}

	err := verifyRuntimeManifest(manifest, root)
	require.Error(t, err)
}
