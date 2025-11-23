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

var testSignature = &object.Signature{
	Name:  "Test User",
	Email: "test@example.com",
	When:  time.Now(),
}

func TestValidateRepository(t *testing.T) {
	t.Run("non-existent repository", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "nonexistent")
		valid, err := ValidateRepository(repoPath)
		assert.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "empty")
		err = os.MkdirAll(repoPath, 0o700)
		require.NoError(t, err)

		valid, err := ValidateRepository(repoPath)
		assert.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("valid repository", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "valid")

		repo, err := git.PlainInit(repoPath, false)
		require.NoError(t, err)

		testFile := filepath.Join(repoPath, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		wt, err := repo.Worktree()
		require.NoError(t, err)

		_, err = wt.Add("test.txt")
		require.NoError(t, err)

		_, err = wt.Commit("Initial commit", &git.CommitOptions{
			Author: testSignature,
		})
		require.NoError(t, err)

		valid, err := ValidateRepository(repoPath)
		assert.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("repository with no commits", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "no-commits")

		_, err = git.PlainInit(repoPath, false)
		require.NoError(t, err)

		valid, err := ValidateRepository(repoPath)
		assert.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("corrupted repository - missing objects dir", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "corrupted")

		repo, err := git.PlainInit(repoPath, false)
		require.NoError(t, err)

		testFile := filepath.Join(repoPath, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o644)
		require.NoError(t, err)

		wt, err := repo.Worktree()
		require.NoError(t, err)

		_, err = wt.Add("test.txt")
		require.NoError(t, err)

		_, err = wt.Commit("Initial commit", &git.CommitOptions{
			Author: testSignature,
		})
		require.NoError(t, err)

		objectsDir := filepath.Join(repoPath, ".git", "objects")
		err = os.RemoveAll(objectsDir)
		require.NoError(t, err)

		valid, err := ValidateRepository(repoPath)
		assert.NoError(t, err)
		assert.False(t, valid)
	})
}

func TestCleanInvalidRepository(t *testing.T) {
	t.Run("clean invalid repository", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "invalid")
		err = os.MkdirAll(repoPath, 0o700)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(repoPath, "somefile.txt"), []byte("test"), 0o644)
		require.NoError(t, err)

		err = CleanInvalidRepository(repoPath)
		assert.NoError(t, err)

		_, err = os.Stat(repoPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("keep valid repository", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "valid")

		repo, err := git.PlainInit(repoPath, false)
		require.NoError(t, err)

		testFile := filepath.Join(repoPath, "test.txt")
		err = os.WriteFile(testFile, []byte("test"), 0o644)
		require.NoError(t, err)

		wt, err := repo.Worktree()
		require.NoError(t, err)

		_, err = wt.Add("test.txt")
		require.NoError(t, err)

		_, err = wt.Commit("Initial commit", &git.CommitOptions{
			Author: testSignature,
		})
		require.NoError(t, err)

		err = CleanInvalidRepository(repoPath)
		assert.NoError(t, err)

		_, err = os.Stat(repoPath)
		assert.NoError(t, err)
	})
}

func TestRepairRepository(t *testing.T) {
	t.Run("repair repository with uncommitted changes", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "sampctl-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		repoPath := filepath.Join(tmpDir, "repair")

		repo, err := git.PlainInit(repoPath, false)
		require.NoError(t, err)

		testFile := filepath.Join(repoPath, "test.txt")
		err = os.WriteFile(testFile, []byte("original"), 0o644)
		require.NoError(t, err)

		wt, err := repo.Worktree()
		require.NoError(t, err)

		_, err = wt.Add("test.txt")
		require.NoError(t, err)

		_, err = wt.Commit("Initial commit", &git.CommitOptions{
			Author: testSignature,
		})
		require.NoError(t, err)

		err = os.WriteFile(testFile, []byte("modified"), 0o644)
		require.NoError(t, err)

		err = RepairRepository(repoPath)
		assert.NoError(t, err)

		content, err := os.ReadFile(testFile)
		require.NoError(t, err)
		assert.Equal(t, "original", string(content))
	})
}
