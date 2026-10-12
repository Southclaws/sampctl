package lockfile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrityContextPreservesHashesAndVerification(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "library.inc"), []byte("stock library() {}"), 0o600))
	previous, err := CalculateDirectoryIntegrity(dir)
	require.NoError(t, err)
	actual, err := CalculateDirectoryIntegrityContext(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, previous, actual)
	assert.Equal(t, "sha256:6f947b0f8092b87b513622113517459beefcd66c439e8c7854f4f5a40d1c7b61", actual)
	ok, err := VerifyIntegrityContext(context.Background(), dir, actual)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIntegrityContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	_, err := CalculateDirectoryIntegrityContext(ctx, dir)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = VerifyIntegrityContext(ctx, dir, "")
	assert.ErrorIs(t, err, context.Canceled)
	err = ValidateWorktreeFilesContext(ctx, dir)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestHashFileContentContextCancelsDuringHashing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.inc")
	require.NoError(t, os.WriteFile(path, make([]byte, 128*1024), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writer := &cancelIntegrityWriter{cancel: cancel}
	err := hashFileContentContext(ctx, writer, path)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, writer.bytes, 128*1024, "cancellation should stop hashing before reading the complete file")
}

func TestIntegrityContextSupportsRegularSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require additional privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "library.inc")
	require.NoError(t, os.WriteFile(target, []byte("stock library() {}"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "library.inc")))
	hash, err := CalculateDirectoryIntegrityContext(context.Background(), dir)
	require.NoError(t, err)
	ok, err := VerifyIntegrityContext(context.Background(), dir, hash)
	require.NoError(t, err)
	assert.True(t, ok)
	require.NoError(t, ValidateWorktreeFilesContext(context.Background(), dir))
	directoryTarget := t.TempDir()
	require.NoError(t, os.Symlink(directoryTarget, filepath.Join(dir, "linked-directory")))
	require.NoError(t, ValidateWorktreeFilesContext(context.Background(), dir))
}

type cancelIntegrityWriter struct {
	cancel context.CancelFunc
	bytes  int
}

func (w *cancelIntegrityWriter) Write(data []byte) (int, error) {
	w.bytes += len(data)
	w.cancel()
	return len(data), nil
}
