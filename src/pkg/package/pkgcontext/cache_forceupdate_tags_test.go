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

func TestPackageContext_ensureRepoExists_forceUpdateFetchesNewTags(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	remoteDir := filepath.Join(tmp, "remote")
	cacheDir := filepath.Join(tmp, "cache")

	repoRemote, err := git.PlainInit(remoteDir, false)
	require.NoError(t, err)

	writeAndCommit := func(filename, contents, msg string) {
		wt, err := repoRemote.Worktree()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(remoteDir, filename), []byte(contents), 0o600))
		_, err = wt.Add(filename)
		require.NoError(t, err)

		_, err = wt.Commit(msg, &git.CommitOptions{Author: &object.Signature{Name: "tester", Email: "tester@example.com", When: time.Now()}})
		require.NoError(t, err)
	}

	writeAndCommit("README.md", "hello", "init")

	pcx := PackageContext{}

	_, err = pcx.ensureRepoExists(remoteDir, cacheDir, "", false, false)
	require.NoError(t, err)

	repoCached, err := git.PlainOpen(cacheDir)
	require.NoError(t, err)

	tags, err := repoCached.Tags()
	require.NoError(t, err)
	defer tags.Close()
	var tagCount int
	err = tags.ForEach(func(_ *plumbing.Reference) error {
		tagCount++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 0, tagCount)

	writeAndCommit("README.md", "hello2", "second")

	head, err := repoRemote.Head()
	require.NoError(t, err)
	_, err = repoRemote.CreateTag("v1.0.0", head.Hash(), nil)
	require.NoError(t, err)

	_, err = pcx.ensureRepoExists(remoteDir, cacheDir, "", false, false)
	require.NoError(t, err)

	repoCached, err = git.PlainOpen(cacheDir)
	require.NoError(t, err)
	tags, err = repoCached.Tags()
	require.NoError(t, err)
	defer tags.Close()
	tagCount = 0
	err = tags.ForEach(func(_ *plumbing.Reference) error {
		tagCount++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 0, tagCount)

	_, err = pcx.ensureRepoExists(remoteDir, cacheDir, "", false, true)
	require.NoError(t, err)

	repoCached, err = git.PlainOpen(cacheDir)
	require.NoError(t, err)
	tags, err = repoCached.Tags()
	require.NoError(t, err)
	defer tags.Close()
	tagCount = 0
	err = tags.ForEach(func(_ *plumbing.Reference) error {
		tagCount++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, tagCount)
}
