package pkgcontext

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublishBuildIncludeLeavesIdenticalFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sampctl_build_file.inc")
	content := []byte("#define SAMPCTL_BUILD_FILE 1\n")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	oldTime := time.Unix(1_700_000_000, 0)
	require.NoError(t, os.Chtimes(path, oldTime, oldTime))
	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, publishBuildInclude(path, content))

	after, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	require.Equal(t, before.ModTime(), after.ModTime())
}

func TestPublishBuildIncludeReplacesContentAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sampctl_build_file.inc")
	require.NoError(t, publishBuildInclude(path, []byte("previous\n")))
	created, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), created.Mode().Perm())
	}
	require.NoError(t, os.Chmod(path, 0o640))
	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, publishBuildInclude(path, []byte("replacement\n")))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement\n", string(content))
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, before.Mode().Perm(), after.Mode().Perm())
	require.False(t, os.SameFile(before, after))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestPublishBuildIncludeRejectsSymlinkAndPreservesTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sampctl_build_file.inc")
	target := filepath.Join(dir, "keep.inc")
	require.NoError(t, os.WriteFile(target, []byte("previous\n"), 0o600))
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create a symlink: %v", err)
		}
		require.NoError(t, err)
	}

	err := publishBuildInclude(path, []byte("replacement\n"))

	require.ErrorContains(t, err, "not a regular file")
	link, err := os.Readlink(path)
	require.NoError(t, err)
	require.Equal(t, target, link)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "previous\n", string(content))
}

func TestPublishBuildIncludeFailurePreservesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permissions require POSIX modes")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sampctl_build_file.inc")
	require.NoError(t, os.WriteFile(path, []byte("previous\n"), 0o600))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o700)) })
	require.NoError(t, os.Chmod(dir, 0o500))

	probe, err := os.CreateTemp(dir, "write-check")
	if err == nil {
		require.NoError(t, probe.Close())
		require.NoError(t, os.Remove(probe.Name()))
		t.Skip("process can write to a read-only directory")
	}
	require.ErrorIs(t, err, os.ErrPermission)

	err = publishBuildInclude(path, []byte("replacement\n"))

	require.ErrorIs(t, err, os.ErrPermission)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "previous\n", string(content))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestPublishBuildIncludeConcurrentReaderSeesCompleteContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sampctl_build_file.inc")
	previous := bytes.Repeat([]byte("previous\n"), 8192)
	replacement := bytes.Repeat([]byte("replacement\n"), 8192)
	require.NoError(t, publishBuildInclude(path, previous))

	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(ready)
		for ctx.Err() == nil {
			content, err := os.ReadFile(path)
			if err != nil {
				done <- err
				return
			}
			if !bytes.Equal(content, previous) && !bytes.Equal(content, replacement) {
				done <- fmt.Errorf("reader saw unexpected content of %d bytes", len(content))
				return
			}
		}
		done <- nil
	}()

	<-ready
	for range 12 {
		require.NoError(t, publishBuildInclude(path, replacement))
		require.NoError(t, publishBuildInclude(path, previous))
	}
	stop()
	require.NoError(t, <-done)
}
