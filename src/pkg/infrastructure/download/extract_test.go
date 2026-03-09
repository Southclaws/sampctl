package download

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldIgnoreFile(t *testing.T) {
	t.Parallel()

	assert.True(t, shouldIgnoreFile(filepath.Join("plugins", "streamer.so"), []string{"*.so"}))
	assert.True(t, shouldIgnoreFile(filepath.Join("plugins", "streamer.so"), []string{"plugins/*"}))
	assert.False(t, shouldIgnoreFile(filepath.Join("plugins", "streamer.so"), []string{"*.dll"}))
	assert.False(t, shouldIgnoreFile(filepath.Join("plugins", "streamer.so"), nil))
}

func TestUntarWithIgnore_Gzip(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "plugin.tar.gz")
	createTarArchive(t, archivePath, true, map[string]string{
		"pkg/plugins/streamer.so":   "new-plugin",
		"pkg/includes/streamer.inc": "#define STREAMER 1\n",
	})

	destDir := t.TempDir()
	ignoredTarget := filepath.Join(destDir, "plugins", "streamer.so")
	require.NoError(t, os.MkdirAll(filepath.Dir(ignoredTarget), 0o755))
	require.NoError(t, os.WriteFile(ignoredTarget, []byte("existing"), 0o644))

	files, err := UntarWithIgnore(archivePath, destDir, map[string]string{
		`pkg/plugins/streamer\.so`:   "plugins/",
		`pkg/includes/streamer\.inc`: "include/",
	}, []string{"*.so"})
	require.NoError(t, err)

	content, err := os.ReadFile(ignoredTarget)
	require.NoError(t, err)
	assert.Equal(t, "existing", string(content))
	assert.FileExists(t, filepath.Join(destDir, "include", "streamer.inc"))
	assert.Equal(t, map[string]string{
		`pkg/includes/streamer\.inc`: filepath.Join(destDir, "include", "streamer.inc"),
	}, files)
}

func TestUntarWithIgnore_ZlibFallback(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "plugin.tar.zz")
	createTarArchive(t, archivePath, false, map[string]string{
		"plugin.txt": "payload",
	})

	destDir := t.TempDir()
	files, err := UntarWithIgnore(archivePath, destDir, map[string]string{"plugin.txt": "copied.txt"}, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"plugin.txt": filepath.Join(destDir, "copied.txt")}, files)
	contents, err := os.ReadFile(filepath.Join(destDir, "copied.txt"))
	require.NoError(t, err)
	assert.Equal(t, "payload", string(contents))
}

func TestUnzipWithIgnore(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "plugin.zip")
	createZipArchive(t, archivePath, map[string]string{
		"pkg/plugins/mysql.dll": "dll",
		"pkg/docs/readme.txt":   "docs",
	})

	destDir := t.TempDir()
	files, err := UnzipWithIgnore(archivePath, destDir, map[string]string{
		`pkg/plugins/mysql\.dll`: "plugins/",
		`pkg/docs/readme\.txt`:   "",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		`pkg/plugins/mysql\.dll`: filepath.Join(destDir, "plugins", "mysql.dll"),
		`pkg/docs/readme\.txt`:   filepath.Join(destDir, "readme.txt"),
	}, files)
}

func TestUntarWithIgnore_InvalidArchive(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "bad.tar")
	require.NoError(t, os.WriteFile(archivePath, []byte("not-an-archive"), 0o644))

	_, err := UntarWithIgnore(archivePath, t.TempDir(), map[string]string{"file.txt": "file.txt"}, nil)
	require.Error(t, err)
}

func TestUnzipWithIgnore_DirectoryAndIgnoredFile(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "with-dir.zip")
	createZipArchiveWithDir(t, archivePath,
		[]string{"pkg/plugins/"},
		map[string]string{
			"pkg/plugins/mysql.dll": "new-dll",
		},
	)

	destDir := t.TempDir()
	ignoredTarget := filepath.Join(destDir, "plugins", "mysql.dll")
	require.NoError(t, os.MkdirAll(filepath.Dir(ignoredTarget), 0o755))
	require.NoError(t, os.WriteFile(ignoredTarget, []byte("existing"), 0o644))

	files, err := UnzipWithIgnore(archivePath, destDir, map[string]string{
		`^pkg/plugins/$`:         "plugins-dir",
		`pkg/plugins/mysql\.dll`: "plugins/",
	}, []string{"*.dll"})
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(destDir, "plugins-dir"))
	data, err := os.ReadFile(ignoredTarget)
	require.NoError(t, err)
	assert.Equal(t, "existing", string(data))
	assert.Empty(t, files)
}

func TestUnzipWithIgnore_InvalidArchive(t *testing.T) {
	t.Parallel()

	archivePath := filepath.Join(t.TempDir(), "bad.zip")
	require.NoError(t, os.WriteFile(archivePath, []byte("not-a-zip"), 0o644))

	_, err := UnzipWithIgnore(archivePath, t.TempDir(), map[string]string{"file.txt": "file.txt"}, nil)
	require.Error(t, err)
}

func TestNameInPaths(t *testing.T) {
	t.Parallel()

	found, source, target := nameInPaths("plugins/test.so", map[string]string{`plugins/.*\.so`: "plugins/"})
	assert.True(t, found)
	assert.Equal(t, `plugins/.*\.so`, source)
	assert.Equal(t, filepath.Join("plugins", "test.so"), target)

	found, source, target = nameInPaths("literal[", map[string]string{"literal[": ""})
	assert.True(t, found)
	assert.Equal(t, "literal[", source)
	assert.Equal(t, "literal[", target)
}

func createTarArchive(t *testing.T, archivePath string, gzipWrapper bool, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	var writer bytes.Buffer
	tw := tar.NewWriter(&writer)
	for name, body := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())

	if gzipWrapper {
		gzw := gzip.NewWriter(f)
		_, err = gzw.Write(writer.Bytes())
		require.NoError(t, err)
		require.NoError(t, gzw.Close())
		return
	}

	zw := zlib.NewWriter(f)
	_, err = zw.Write(writer.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
}

func createZipArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}

func createZipArchiveWithDir(t *testing.T, archivePath string, dirs []string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	zw := zip.NewWriter(f)
	for _, name := range dirs {
		h := &zip.FileHeader{Name: name}
		h.SetMode(os.ModeDir | 0o755)
		_, err := zw.CreateHeader(h)
		require.NoError(t, err)
	}
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}
