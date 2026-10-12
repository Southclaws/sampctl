package lockfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitMetadataPreflightSupportsGitdirFile(t *testing.T) {
	repo, commit := seedGitRepo(t, map[string]string{"main.pwn": "main() {}"}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)
	dir := wt.Filesystem.Root()
	metadata := filepath.Join(t.TempDir(), "metadata")
	require.NoError(t, os.Rename(filepath.Join(dir, ".git"), metadata))
	relative, err := filepath.Rel(dir, metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+relative+"\n"), 0o600))
	require.NoError(t, ValidateWorktreeFilesContext(context.Background(), dir))
	ok, err := VerifyIntegrityContext(context.Background(), dir, CalculateCommitIntegrity(commit))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestGitMetadataPreflightRejectsInvalidGitdirFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("not a gitdir pointer"), 0o600))
	require.Error(t, ValidateWorktreeFilesContext(context.Background(), dir))
}
