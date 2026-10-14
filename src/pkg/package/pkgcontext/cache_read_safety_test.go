package pkgcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage"
	"github.com/gofrs/flock"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestDependencyRecloneLocksSharedSourceThroughoutClone(t *testing.T) {
	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dependency"}
	cachePath := meta.CachePath(cacheDir)
	cacheFixtureRepository(t, cachePath, "")

	lockPath := filepath.Join(filepath.Dir(cachePath), "."+filepath.Base(cachePath)+".sampctl.lock")
	store := &fakeRepositoryStore{
		openFn: git.PlainOpen,
		cloneFn: func(path string, bare bool, opts *git.CloneOptions) (*git.Repository, error) {
			lock := flock.New(lockPath)
			locked, err := lock.TryLock()
			require.NoError(t, err)

			require.NoError(t, lock.Close())
			if locked {
				t.Error("shared source cache is unlocked while cloning")
			}

			return git.PlainClone(path, bare, opts)
		},
	}

	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir, RepoStore: store}}
	destination := filepath.Join(t.TempDir(), "dependency")

	_, err := pcx.recloneDependencyContext(context.Background(), meta, destination)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(destination, "include.inc"))
	require.Equal(t, 1, store.cloneCalls)
}

func TestCachedTagLookupRejectsMetadataSymlink(t *testing.T) {
	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dependency"}
	cachePath := meta.CachePath(cacheDir)
	repo := cacheFixtureRepository(t, cachePath, "")

	head, err := repo.Head()
	require.NoError(t, err)

	_, err = repo.CreateTag("1.0.0", head.Hash(), nil)
	require.NoError(t, err)

	external := filepath.Join(t.TempDir(), "metadata")
	require.NoError(t, os.Rename(filepath.Join(cachePath, ".git"), external))
	if err := os.Symlink(external, filepath.Join(cachePath, ".git")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}
	_, err = pcx.latestTagFromCache(meta)
	require.ErrorIs(t, err, os.ErrInvalid)
}

func TestLatestTagLookupWaitsForCacheLockAndHonorsCancellation(t *testing.T) {
	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dependency"}
	cachePath := meta.CachePath(cacheDir)
	repo := cacheFixtureRepository(t, cachePath, "")
	head, err := repo.Head()
	require.NoError(t, err)

	_, err = repo.CreateTag("1.0.0", head.Hash(), nil)
	require.NoError(t, err)

	unlock, err := lockRepositoryPath(t.Context(), cachePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unlock()) })

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}

	_, err = pcx.resolveLatestTag(ctx, meta, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("resolveLatestTag(locked source) = %v, want cancellation", err)
	}
}

type cacheConfigFailure struct {
	storage.Storer
}

func (cacheConfigFailure) Config() (*gitconfig.Config, error) {
	return nil, os.ErrPermission
}

func TestCacheOriginPermissionFailureIsNotMistakenForMissingRemote(t *testing.T) {
	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dependency"}
	cachePath := meta.CachePath(cacheDir)
	repo := cacheFixtureRepository(t, cachePath, "")
	repo.Storer = cacheConfigFailure{Storer: repo.Storer}
	pcx := PackageContext{PackageServices: PackageServices{
		CacheDir: cacheDir,
		RepoStore: &fakeRepositoryStore{openFn: func(string) (*git.Repository, error) {
			return repo, nil
		}},
	}}

	noOrigin, err := pcx.cachedRepoHasNoOriginContext(t.Context(), meta)
	require.ErrorIs(t, err, os.ErrPermission)
	require.False(t, noOrigin)
}
