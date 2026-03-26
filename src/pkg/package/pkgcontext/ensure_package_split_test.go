package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestRemoveInvalidDependencyRepoRemovesInvalidRepo(t *testing.T) {
	t.Parallel()

	depDir := filepath.Join(t.TempDir(), "dep")
	require.NoError(t, os.MkdirAll(depDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(depDir, "stale.txt"), []byte("stale"), 0o644))

	health := &fakeRepositoryHealth{
		validateFn: func(path string) (bool, error) {
			assert.Equal(t, depDir, path)
			return false, nil
		},
		repairFn: func(string) error { return nil },
	}

	pcx := &PackageContext{PackageServices: PackageServices{RepoHealth: health}}
	err := pcx.removeInvalidDependencyRepo(versioning.DependencyMeta{Repo: "dep"}, depDir)
	require.NoError(t, err)
	assert.Equal(t, 1, health.validateCalls)
	assert.False(t, fs.Exists(depDir))
}

func TestRemoveInvalidDependencyRepoLeavesValidRepo(t *testing.T) {
	t.Parallel()

	depDir := filepath.Join(t.TempDir(), "dep")
	require.NoError(t, os.MkdirAll(depDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(depDir, "keep.txt"), []byte("keep"), 0o644))

	health := &fakeRepositoryHealth{
		validateFn: func(path string) (bool, error) {
			assert.Equal(t, depDir, path)
			return true, nil
		},
		repairFn: func(string) error { return nil },
	}

	pcx := &PackageContext{PackageServices: PackageServices{RepoHealth: health}}
	err := pcx.removeInvalidDependencyRepo(versioning.DependencyMeta{Repo: "dep"}, depDir)
	require.NoError(t, err)
	assert.Equal(t, 1, health.validateCalls)
	assert.True(t, fs.Exists(filepath.Join(depDir, "keep.txt")))
}

func TestRecordDependencyResolutionTransitive(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("ok"), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com"}})
	require.NoError(t, err)

	resolver := &fakeDependencyLock{lockfile: lockfile.New("dev")}
	pcx := &PackageContext{
		Package:              pawnpackage.Package{Repo: "root"},
		PackageLockfileState: PackageLockfileState{lockfileResolver: resolver},
	}

	meta := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "dep", Tag: "v1.0.0"}
	pcx.recordDependencyResolution(meta, "parent/repo", repo)

	assert.Equal(t, meta, resolver.lastResolutionIn)
	assert.True(t, resolver.lastTransitive)
	assert.Equal(t, "parent/repo", resolver.lastRequiredBy)
	assert.NotEmpty(t, resolver.lastResolution.Commit)
	assert.Equal(t, "v1.0.0", resolver.lastResolution.Resolved)
}
