package pkgcontext

import (
	"context"
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
)

func TestUpdateLockfileRecordsRequiredByForTransitiveDependencies(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	seedLockfileRepo(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-b", Tag: "1.0.0"}, `{"entry":"libb.pwn","output":"gamemodes/libb.amx"}`)
	seedLockfileRepo(t, cacheDir, versioning.DependencyMeta{User: "user", Repo: "lib-a", Tag: "1.0.0"}, `{"entry":"liba.pwn","output":"gamemodes/liba.amx","dependencies":["user/lib-b:1.0.0"]}`)

	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), []byte(`{"entry":"main.pwn","output":"gamemodes/main.amx","dependencies":["user/lib-a:1.0.0"]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "main.pwn"), []byte("main() {}"), 0o644))

	pcx, err := NewPackageContext(NewPackageContextOptions{Parent: true, Dir: projectDir, Platform: "linux", CacheDir: cacheDir})
	require.NoError(t, err)
	require.NoError(t, pcx.InitLockfileResolver("dev"))

	require.NoError(t, pcx.UpdateLockfile(context.Background(), DependencyUpdateRequest{}))

	lf := pcx.GetLockfile()
	require.NotNil(t, lf)

	libA, ok := lf.GetDependency(lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-a"}))
	require.True(t, ok)
	assert.False(t, libA.Transitive)

	libB, ok := lf.GetDependency(lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-b"}))
	require.True(t, ok)
	assert.True(t, libB.Transitive)
	assert.Equal(t, []string{lockfile.DependencyKey(versioning.DependencyMeta{User: "user", Repo: "lib-a"})}, libB.RequiredBy)
}

func seedLockfileRepo(t *testing.T, cacheDir string, meta versioning.DependencyMeta, pawnJSON string) plumbing.Hash {
	t.Helper()

	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), []byte(pawnJSON), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, meta.Repo+".pwn"), []byte("main() {}"), 0o644))
	_, err = wt.Add("pawn.json")
	require.NoError(t, err)
	_, err = wt.Add(meta.Repo + ".pwn")
	require.NoError(t, err)

	hash, err := wt.Commit("fixture", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	require.NoError(t, err)
	_, err = repo.CreateTag(meta.Tag, hash, nil)
	require.NoError(t, err)

	return hash
}
