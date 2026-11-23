package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRepository(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonexistent")

		valid, err := ValidateRepository(path)

		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("empty directory", func(t *testing.T) {
		path := t.TempDir()

		valid, err := ValidateRepository(path)

		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("repository with a commit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "valid")
		commitRepositoryTestFile(t, path, "test content")

		valid, err := ValidateRepository(path)

		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("repository with no commits", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "no-commits")
		_, err := git.PlainInit(path, false)
		require.NoError(t, err)

		valid, err := ValidateRepository(path)

		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("repository missing its objects directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "corrupted")
		commitRepositoryTestFile(t, path, "test")
		require.NoError(t, os.RemoveAll(filepath.Join(path, ".git", "objects")))

		valid, err := ValidateRepository(path)

		require.NoError(t, err)
		assert.False(t, valid)
	})
}

func TestCleanInvalidRepositoryRemovesRepositoryWithoutCommits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid")
	_, err := git.PlainInit(path, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(path, "somefile.txt"), []byte("test"), 0o644))

	err = CleanInvalidRepository(path)

	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestCleanInvalidRepositoryKeepsCommittedRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid")
	commitRepositoryTestFile(t, path, "test")

	err := CleanInvalidRepository(path)

	require.NoError(t, err)
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestRepairRepositoryRestoresCommittedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repair")
	file := commitRepositoryTestFile(t, path, "original")
	require.NoError(t, os.WriteFile(file, []byte("modified"), 0o644))

	err := RepairRepository(path)

	require.NoError(t, err)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
}

func commitRepositoryTestFile(t *testing.T, path, contents string) string {
	t.Helper()

	repo, err := git.PlainInit(path, false)
	require.NoError(t, err)
	file := filepath.Join(path, "test.txt")
	require.NoError(t, os.WriteFile(file, []byte(contents), 0o644))

	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("test.txt")
	require.NoError(t, err)
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Unix(1, 0)},
	})
	require.NoError(t, err)
	return file
}
