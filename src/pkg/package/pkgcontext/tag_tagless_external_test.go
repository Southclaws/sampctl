package pkgcontext_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestTagTaglessDependencies_PinsLatestTagFromCache(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	depUser := "testuser"
	depRepo := "testrepo"
	depMeta := versioning.DependencyMeta{User: depUser, Repo: depRepo}
	cachePath := depMeta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	write := func(name, contents string) {
		require.NoError(t, os.WriteFile(filepath.Join(cachePath, name), []byte(contents), 0o644))
		_, err = wt.Add(name)
		require.NoError(t, err)
	}

	commit := func(msg string, when time.Time) string {
		hash, err := wt.Commit(msg, &git.CommitOptions{
			Author:    &object.Signature{Name: "test", Email: "test@example.com", When: when},
			Committer: &object.Signature{Name: "test", Email: "test@example.com", When: when},
		})
		require.NoError(t, err)
		return hash.String()
	}

	write("file.txt", "one")
	write("pawn.json", "{}")
	_ = commit("c1", time.Unix(100, 0))
	_, err = repo.CreateTag("1.0.0", headHash(repo, t), nil)
	require.NoError(t, err)

	write("file.txt", "two")
	_ = commit("c2", time.Unix(200, 0))
	_, err = repo.CreateTag("2.0.0", headHash(repo, t), nil)
	require.NoError(t, err)

	cfg := map[string]any{
		"entry":        "test.pwn",
		"output":       "test.amx",
		"dependencies": []string{depUser + "/" + depRepo},
	}
	b, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), b, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux"}

	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.NoError(t, err)
	require.True(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)
	var final map[string]any
	require.NoError(t, json.Unmarshal(finalBytes, &final))

	depsAny := final["dependencies"].([]any)
	require.Equal(t, []any{depUser + "/" + depRepo + ":2.0.0"}, depsAny)
}

func headHash(repo *git.Repository, t *testing.T) plumbing.Hash {
	head, err := repo.Head()
	require.NoError(t, err)
	return head.Hash()
}
