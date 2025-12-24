package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestLatestTagFromCache_TieBreaksNonSemverByName(t *testing.T) {
	cacheDir := t.TempDir()
	pcx := &PackageContext{CacheDir: cacheDir}

	meta := versioning.DependencyMeta{User: "testuser", Repo: "testrepo"}
	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "file.txt"), []byte("one"), 0o644))
	_, err = wt.Add("file.txt")
	require.NoError(t, err)
	_, err = wt.Commit("c1", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	require.NoError(t, err)

	head, err := repo.Head()
	require.NoError(t, err)

	_, err = repo.CreateTag("latest", head.Hash(), nil)
	require.NoError(t, err)
	_, err = repo.CreateTag("stable", head.Hash(), nil)
	require.NoError(t, err)

	tag, err := pcx.latestTagFromCache(meta)
	require.NoError(t, err)
	require.Equal(t, "stable", tag)
}
