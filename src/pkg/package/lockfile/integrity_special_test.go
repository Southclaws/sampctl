//go:build linux || darwin

package lockfile

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrityContextRejectsSpecialRelevantFiles(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "fifo", true: "symlink to fifo"}[symlink], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "library.inc")
			if symlink {
				target := filepath.Join(t.TempDir(), "user-pipe")
				require.NoError(t, syscall.Mkfifo(target, 0o600))
				require.NoError(t, os.Symlink(target, path))
			} else {
				require.NoError(t, syscall.Mkfifo(path, 0o600))
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err := CalculateDirectoryIntegrityContext(ctx, dir)
			require.Error(t, err)
			assert.NoError(t, ctx.Err(), "special files must fail before waiting for cancellation")
		})
	}
}

func TestCommitIntegrityContextRejectsSpecialWorktreeFiles(t *testing.T) {
	repo, commit := seedGitRepo(t, map[string]string{"main.pwn": "main() {}"}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)

	dir := wt.Filesystem.Root()
	path := filepath.Join(dir, "untracked-pipe.txt")
	require.NoError(t, syscall.Mkfifo(path, 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = VerifyIntegrityContext(ctx, dir, CalculateCommitIntegrity(commit))
	require.Error(t, err)
	assert.NoError(t, ctx.Err(), "Git status must not open special worktree files")

	_, err = CalculateDirectoryIntegrityContext(ctx, dir)
	require.NoError(t, err, "nonrelevant files retain their directory integrity semantics")
}

func TestWorktreePreflightChecksSymlinkedRootAndHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, ".hidden-pipe"), 0o600))
	root := filepath.Join(t.TempDir(), "dependency")
	require.NoError(t, os.Symlink(dir, root))

	err := ValidateWorktreeFilesContext(context.Background(), root)
	assert.Error(t, err)
}

func TestWorktreePreflightRejectsSpecialGitMetadata(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o700))
	require.NoError(t, syscall.Mkfifo(filepath.Join(gitDir, "metadata-pipe"), 0o600))

	require.Error(t, ValidateWorktreeFilesContext(context.Background(), dir))
}

func TestWorktreePreflightSkipsSymlinkDirectoryContents(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(target, "external-pipe"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "linked-directory")))

	require.NoError(t, ValidateWorktreeFilesContext(context.Background(), dir))
}

func TestGitMetadataPreflightRejectsBlockingFiles(t *testing.T) {
	for _, name := range []string{".git", ".git/HEAD", ".git/config", ".git/objects/ab/object"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, filepath.FromSlash(name))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, syscall.Mkfifo(path, 0o600))

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			require.Error(t, ValidateWorktreeFilesContext(ctx, dir))
			assert.NoError(t, ctx.Err(), "Git metadata must fail before opening a special file")
		})
	}
}

func TestGitMetadataPreflightInspectsExternalDirectories(t *testing.T) {
	for _, layout := range []string{"gitdir file", "gitdir symlink", "objects symlink", "object alternates"} {
		t.Run(layout, func(t *testing.T) {
			dir := t.TempDir()
			metadata := t.TempDir()
			require.NoError(t, syscall.Mkfifo(filepath.Join(metadata, "pipe"), 0o600))
			switch layout {
			case "gitdir file":
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+metadata+"\n"), 0o600))
			case "gitdir symlink":
				require.NoError(t, os.Symlink(metadata, filepath.Join(dir, ".git")))
			case "objects symlink":
				require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o700))
				require.NoError(t, os.Symlink(metadata, filepath.Join(dir, ".git", "objects")))
			case "object alternates":
				require.NoError(t, os.Mkdir(filepath.Join(metadata, "objects"), 0o700))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects", "info"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "objects", "info", "alternates"), []byte(filepath.Join(metadata, "objects")+"\n"), 0o600))
			}

			require.Error(t, ValidateWorktreeFilesContext(context.Background(), dir))
		})
	}
}

func TestCommitIntegrityPreflightRunsBeforeOpeningMetadata(t *testing.T) {
	repo, commit := seedGitRepo(t, map[string]string{"main.pwn": "main() {}"}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)

	dir := wt.Filesystem.Root()
	path := filepath.Join(dir, ".git", "config")
	require.NoError(t, os.Remove(path))
	require.NoError(t, syscall.Mkfifo(path, 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = VerifyIntegrityContext(ctx, dir, CalculateCommitIntegrity(commit))
	require.Error(t, err)
	assert.NoError(t, ctx.Err())
}

func TestGitMetadataPreflightInspectsCommonDirectory(t *testing.T) {
	for _, special := range []string{"pointer", "common metadata"} {
		t.Run(special, func(t *testing.T) {
			dir := t.TempDir()
			metadata := filepath.Join(dir, ".git")
			require.NoError(t, os.Mkdir(metadata, 0o755))
			pointer := filepath.Join(metadata, "commondir")
			if special == "pointer" {
				require.NoError(t, syscall.Mkfifo(pointer, 0o600))
			} else {
				common := t.TempDir()
				require.NoError(t, syscall.Mkfifo(filepath.Join(common, "HEAD"), 0o600))
				require.NoError(t, os.WriteFile(pointer, []byte(common+"\n"), 0o600))
			}

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			require.Error(t, ValidateWorktreeFilesContext(ctx, dir))
			assert.NoError(t, ctx.Err())
		})
	}
}
