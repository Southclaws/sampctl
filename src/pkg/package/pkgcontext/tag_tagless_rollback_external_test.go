package pkgcontext_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestTagTaglessDependencies_RollsBackOnCacheRefreshFailure(t *testing.T) {
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

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), []byte("{\n\t\"dependencies\": [\"other/dep\"]\n}\n"), 0o644))
	_, err = wt.Add("pawn.json")
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
	_, err = repo.CreateTag("1.2.3", head.Hash(), nil)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(cacheDir, 0o555))

	cfg := map[string]any{
		"entry":        "test.pwn",
		"output":       "test.amx",
		"dependencies": []string{depUser + "/" + depRepo},
	}
	initialBytes, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), initialBytes, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux"}

	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.Error(t, err)
	require.False(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)

	var final map[string]any
	require.NoError(t, json.Unmarshal(finalBytes, &final))

	depsAny := final["dependencies"].([]any)
	require.Equal(t, []any{depUser + "/" + depRepo}, depsAny)
}
