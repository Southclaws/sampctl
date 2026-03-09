package download

import (
	"archive/tar"
	"compress/gzip"
	"compress/zlib"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestUntarAllPreserveLayout_IgnoresAppleDoubleMetadata(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "compiler.tar.gz")
	require.NoError(t, writeTarGzArchiveForTest(archive, map[string]string{
		"._pawnc-3.10.8-linux":                 "metadata",
		"pawnc-3.10.8-linux/._bin":             "metadata",
		"pawnc-3.10.8-linux/bin/._pawncc":      "metadata",
		"pawnc-3.10.8-linux/lib/._libpawnc.so": "metadata",
		"pawnc-3.10.8-linux/bin/pawncc":        "exe",
		"pawnc-3.10.8-linux/lib/libpawnc.so":   "lib",
	}))

	dir := t.TempDir()
	files, err := UntarAllPreserveLayout(archive, dir)
	require.NoError(t, err)

	assert.True(t, isArchiveMetadataPath("._pawnc-3.10.8-linux"))
	assert.True(t, isArchiveMetadataPath("pawnc-3.10.8-linux/bin/._pawncc"))
	assert.False(t, isArchiveMetadataPath("pawnc-3.10.8-linux/bin/pawncc"))
	assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc")))
	assert.True(t, fs.Exists(filepath.Join(dir, "lib", "libpawnc.so")))
	assert.False(t, fs.Exists(filepath.Join(dir, "pawnc-3.10.8-linux")))
	assert.NotContains(t, files, "._pawnc-3.10.8-linux")
	assert.Equal(t, filepath.Join(dir, "bin", "pawncc"), files["pawnc-3.10.8-linux/bin/pawncc"])
}

func TestUntarAllPreserveLayout_SupportsZlibCompressedTar(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "compiler.tar.zlib")
	require.NoError(t, writeTarZlibArchiveForTest(archive, map[string]string{
		"pawnc/bin/pawncc":      "exe",
		"pawnc/lib/libpawnc.so": "lib",
	}))

	dir := t.TempDir()
	files, err := UntarAllPreserveLayout(archive, dir)
	require.NoError(t, err)

	assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc")))
	assert.True(t, fs.Exists(filepath.Join(dir, "lib", "libpawnc.so")))
	assert.Equal(t, filepath.Join(dir, "bin", "pawncc"), files["pawnc/bin/pawncc"])
}

func TestCleanArchivePath_NormalizesWindowsPaths(t *testing.T) {
	t.Parallel()

	cleaned, ok := cleanArchivePath(`bin\pawncc`)
	require.True(t, ok)
	assert.Equal(t, "bin/pawncc", cleaned)

	_, ok = cleanArchivePath(`..\evil`)
	assert.False(t, ok)

	_, ok = cleanArchivePath(`C:\pawncc`)
	assert.False(t, ok)

	_, ok = cleanArchivePath(`\\server\share\pawncc`)
	assert.False(t, ok)
}

func writeTarGzArchiveForTest(filename string, files map[string]string) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	gzw := gzip.NewWriter(out)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}

func writeTarZlibArchiveForTest(filename string, files map[string]string) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zlib.NewWriter(out)
	defer zw.Close()

	tw := tar.NewWriter(zw)
	defer tw.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}
