package fs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "file.txt")
	require.NoError(t, WriteFileAtomic(path, []byte("hello"), PermDirShared, PermFilePrivate))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, PermFilePrivate, info.Mode().Perm())
}

func TestWriteFromReaderAtomic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "reader.txt")
	require.NoError(t, WriteFromReaderAtomic(path, bytes.NewBufferString("payload"), PermDirPrivate, PermFileShared))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(data))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, PermFileShared, info.Mode().Perm())
}

func TestWriteJSONAtomic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, WriteJSONAtomic(path, map[string]any{"name": "value"}, PermDirShared, PermFileShared))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "\"name\": \"value\"")
	assert.True(t, bytes.HasSuffix(data, []byte("\n")))
}

func TestEnsureDirHelpers(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "a", "b")
	require.NoError(t, EnsureDir(dir, PermDirShared))
	assert.DirExists(t, dir)

	path := filepath.Join(t.TempDir(), "c", "d", "file.txt")
	require.NoError(t, EnsureDirForFile(path, PermDirPrivate))
	assert.DirExists(t, filepath.Dir(path))
}

func TestEnsurePackageLayout(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	require.NoError(t, EnsurePackageLayout(base, false))
	assert.DirExists(t, filepath.Join(base, "gamemodes"))
	assert.DirExists(t, filepath.Join(base, "filterscripts"))
	assert.DirExists(t, filepath.Join(base, "scriptfiles"))
	assert.DirExists(t, filepath.Join(base, "plugins"))
	assert.DirExists(t, filepath.Join(base, "npcmodes"))
	assert.NoDirExists(t, filepath.Join(base, "components"))

	openmp := t.TempDir()
	require.NoError(t, EnsurePackageLayout(openmp, true))
	assert.DirExists(t, filepath.Join(openmp, "components"))
}

func TestPathHelpers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, filepath.Join("a", "b"), Join("a", "b"))

	abs, err := Abs(".")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(abs))
	assert.True(t, filepath.IsAbs(MustAbs(".")))

	assert.Equal(t, "rel-target.txt", Rel("rel-target.txt"))

	path := filepath.Join(t.TempDir(), "exists.txt")
	require.NoError(t, os.WriteFile(path, []byte("ok"), 0o644))
	assert.True(t, Exists(path))
	assert.False(t, Exists(filepath.Join(t.TempDir(), "missing.txt")))
}

func TestChmodHelpers(t *testing.T) {
	t.Parallel()

	pathA := filepath.Join(t.TempDir(), "a.txt")
	pathB := filepath.Join(t.TempDir(), "b.txt")
	require.NoError(t, os.WriteFile(pathA, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(pathB, []byte("b"), 0o644))

	files := map[string]string{"a": pathA, "b": pathB}
	require.NoError(t, ChmodAll(files, PermFileExec))

	infoA, err := os.Stat(pathA)
	require.NoError(t, err)
	assert.Equal(t, PermFileExec, infoA.Mode().Perm())

	infoB, err := os.Stat(pathB)
	require.NoError(t, err)
	assert.Equal(t, PermFileExec, infoB.Mode().Perm())

	assert.True(t, IsPosixPlatform("linux"))
	assert.True(t, IsPosixPlatform("darwin"))
	assert.False(t, IsPosixPlatform("windows"))

	require.NoError(t, ChmodAllIfPosix("windows", files, PermFilePrivate))
	infoA, err = os.Stat(pathA)
	require.NoError(t, err)
	assert.Equal(t, PermFileExec, infoA.Mode().Perm())
}

func TestConfigDirHelpers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	dir, err := ConfigDir()
	require.NoError(t, err)
	assert.DirExists(t, dir)
	assert.Equal(t, dir, MustConfigDir())
	assert.Equal(t, configFolderName, filepath.Base(dir))
}
