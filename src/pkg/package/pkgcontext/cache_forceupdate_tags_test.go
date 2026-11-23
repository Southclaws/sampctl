package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"
)

func TestEnsureRepoExistsFetchesNewTagsOnlyOnForceUpdate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote")
	cacheDir := filepath.Join(root, "cache")
	remote, err := git.PlainInit(remoteDir, false)
	require.NoError(t, err)
	worktree, err := remote.Worktree()
	require.NoError(t, err)

	writeAndCommit := func(contents, message string) plumbing.Hash {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "README.md"), []byte(contents), 0o600))
		_, err := worktree.Add("README.md")
		require.NoError(t, err)
		commit, err := worktree.Commit(message, &git.CommitOptions{
			Author: &object.Signature{Name: "tester", Email: "tester@example.com", When: time.Unix(100, 0)},
		})
		require.NoError(t, err)
		return commit
	}
	writeAndCommit("hello", "initial commit")
	pcx := PackageContext{}
	request := repoEnsureRequest{From: remoteDir, To: cacheDir}

	_, err = pcx.ensureRepoExists(request)

	require.NoError(t, err)
	require.Empty(t, cachedRepositoryTags(t, cacheDir))

	release := writeAndCommit("hello2", "release commit")
	_, err = remote.CreateTag("v1.0.0", release, nil)
	require.NoError(t, err)

	_, err = pcx.ensureRepoExists(request)

	require.NoError(t, err)
	require.Empty(t, cachedRepositoryTags(t, cacheDir))

	request.ForceUpdate = true
	_, err = pcx.ensureRepoExists(request)

	require.NoError(t, err)
	require.Equal(t, []string{"v1.0.0"}, cachedRepositoryTags(t, cacheDir))
}

func cachedRepositoryTags(t *testing.T, directory string) []string {
	t.Helper()

	repository, err := git.PlainOpen(directory)
	require.NoError(t, err)
	tags, err := repository.Tags()
	require.NoError(t, err)
	defer tags.Close()

	var names []string
	err = tags.ForEach(func(tag *plumbing.Reference) error {
		names = append(names, tag.Name().Short())
		return nil
	})
	require.NoError(t, err)
	return names
}
