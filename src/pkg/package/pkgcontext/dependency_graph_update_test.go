package pkgcontext

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsureProjectRefreshesTransitiveDependenciesForUpdatedDirectDependency(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	seedLockfileRepo(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-b", Tag: "1.0.0"}, `{"entry":"libb.pwn","output":"gamemodes/libb.amx"}`)
	seedLockfileRepo(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-c", Tag: "1.0.0"}, `{"entry":"libc.pwn","output":"gamemodes/libc.amx"}`)
	seedStaleCachedLatestDependency(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-a"},
		`{"entry":"liba.pwn","output":"gamemodes/liba.amx","dependencies":["user/lib-b:1.0.0"]}`,
		`{"entry":"liba.pwn","output":"gamemodes/liba.amx","dependencies":["user/lib-c:1.0.0"]}`,
	)

	rootConfig := map[string]any{
		"entry":        "main.pwn",
		"output":       "gamemodes/main.amx",
		"dependencies": []string{"user/lib-a:latest"},
		"runtime": map[string]any{
			"version": "0.3.7",
		},
	}
	configBytes, err := json.MarshalIndent(rootConfig, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), configBytes, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "main.pwn"), []byte("main() {}"), 0o644))

	seedStagedRuntime(t, cacheDir, runtimecfg.Runtime{Version: "0.3.7", Platform: "linux"})

	pcx, err := NewPackageContext(NewPackageContextOptions{
		Parent:   true,
		Dir:      projectDir,
		Platform: "linux",
		CacheDir: cacheDir,
	})
	require.NoError(t, err)
	require.NoError(t, pcx.InitLockfileResolver("dev"))

	updated, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{Enabled: true})
	require.NoError(t, err)
	assert.False(t, updated)

	require.DirExists(t, filepath.Join(projectDir, "dependencies", "lib-c"))
	_, err = os.Stat(filepath.Join(projectDir, "dependencies", "lib-b"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	lf, err := lockfile.Load(projectDir)
	require.NoError(t, err)
	require.NotNil(t, lf)
	assert.Contains(t, lf.Dependencies, lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-c"}))
	assert.NotContains(t, lf.Dependencies, lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-b"}))
}

func TestUpdateLockfileRefreshesTransitiveDependenciesForUpdatedDirectDependency(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	seedLockfileRepo(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-b", Tag: "1.0.0"}, `{"entry":"libb.pwn","output":"gamemodes/libb.amx"}`)
	seedLockfileRepo(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-c", Tag: "1.0.0"}, `{"entry":"libc.pwn","output":"gamemodes/libc.amx"}`)
	seedStaleCachedLatestDependency(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-a"},
		`{"entry":"liba.pwn","output":"gamemodes/liba.amx","dependencies":["user/lib-b:1.0.0"]}`,
		`{"entry":"liba.pwn","output":"gamemodes/liba.amx","dependencies":["user/lib-c:1.0.0"]}`,
	)

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), []byte(`{"entry":"main.pwn","output":"gamemodes/main.amx","dependencies":["user/lib-a:latest"]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "main.pwn"), []byte("main() {}"), 0o644))

	pcx, err := NewPackageContext(NewPackageContextOptions{Parent: true, Dir: projectDir, Platform: "linux", CacheDir: cacheDir})
	require.NoError(t, err)
	require.NoError(t, pcx.InitLockfileResolver("dev"))

	require.NoError(t, pcx.UpdateLockfile(context.Background(), DependencyUpdateRequest{Enabled: true}))

	lf := pcx.GetLockfile()
	require.NotNil(t, lf)
	assert.Contains(t, lf.Dependencies, lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-c"}))
	assert.NotContains(t, lf.Dependencies, lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-b"}))
}

func seedStaleCachedLatestDependency(
	t *testing.T,
	cacheDir string,
	meta versioning.DependencyMeta,
	initialDefinition string,
	updatedDefinition string,
) {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), meta.Repo+"-source")
	repo, err := git.PlainInit(sourcePath, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)

	writeDefinition := func(definition string) plumbing.Hash {
		t.Helper()

		require.NoError(t, os.WriteFile(filepath.Join(sourcePath, "pawn.json"), []byte(definition), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(sourcePath, meta.Repo+".pwn"), []byte("main() {}"), 0o644))
		_, err = worktree.Add("pawn.json")
		require.NoError(t, err)
		_, err = worktree.Add(meta.Repo + ".pwn")
		require.NoError(t, err)

		hash, commitErr := worktree.Commit("fixture", &git.CommitOptions{
			Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(time.Now().Unix(), 0)},
			Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(time.Now().Unix(), 0)},
		})
		require.NoError(t, commitErr)
		return hash
	}

	firstHash := writeDefinition(initialDefinition)
	_, err = repo.CreateTag("1.0.0", firstHash, nil)
	require.NoError(t, err)

	secondHash := writeDefinition(updatedDefinition)
	_, err = repo.CreateTag("2.0.0", secondHash, nil)
	require.NoError(t, err)

	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(cachePath), 0o755))
	_, err = git.PlainClone(cachePath, false, &git.CloneOptions{URL: sourcePath})
	require.NoError(t, err)

	cachedRepo, err := git.PlainOpen(cachePath)
	require.NoError(t, err)
	cachedWorktree, err := cachedRepo.Worktree()
	require.NoError(t, err)
	require.NoError(t, cachedWorktree.Reset(&git.ResetOptions{
		Commit: firstHash,
		Mode:   git.HardReset,
	}))
}
