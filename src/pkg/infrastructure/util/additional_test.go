package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFileErrors(t *testing.T) {
	t.Run("rejects non regular source", func(t *testing.T) {
		srcDir := t.TempDir()
		err := CopyFile(srcDir, filepath.Join(t.TempDir(), "out"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-regular source file")
	})

	t.Run("rejects non regular destination", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

		dstDir := t.TempDir()
		err := CopyFile(src, dstDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-regular destination file")
	})

	t.Run("same file is a no op", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
		require.NoError(t, CopyFile(path, path))
	})
}

func TestCopyFileContents(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0o644))

	require.NoError(t, copyFileContents(src, dst))
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(data))
}

func TestPathHelpers(t *testing.T) {
	abs := FullPath(".")
	assert.True(t, filepath.IsAbs(abs))

	base := FullPath(filepath.Dir(os.Args[0]))
	target := filepath.Join(base, "child.txt")
	assert.Equal(t, "child.txt", RelPath(target))
}

func TestExistsAndDirEmpty(t *testing.T) {
	file := filepath.Join(t.TempDir(), "exists.txt")
	require.NoError(t, os.WriteFile(file, []byte("ok"), 0o644))
	assert.True(t, Exists(file))
	assert.False(t, Exists(filepath.Join(t.TempDir(), "missing.txt")))

	emptyDir := t.TempDir()
	assert.True(t, DirEmpty(emptyDir))
	require.NoError(t, os.WriteFile(filepath.Join(emptyDir, "item.txt"), []byte("x"), 0o644))
	assert.False(t, DirEmpty(emptyDir))
}

func TestGetConfigDir(t *testing.T) {
	dir := GetConfigDir()
	assert.NotEmpty(t, dir)
	assert.Equal(t, FolderName, filepath.Base(dir))
}
