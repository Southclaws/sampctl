package pkgcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func setCacheOrigin(t *testing.T, repo *git.Repository, origin string) {
	t.Helper()
	_, err := repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{origin}})
	require.NoError(t, err)
}

func TestRepositoryCacheSeparatesHosts(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}
	for _, host := range []string{"github.com", "gitlab.com"} {
		meta := versioning.DependencyMeta{Site: host, User: "fixture", Repo: "shared"}
		repo := cacheFixtureRepository(t, meta.CachePath(cacheDir), `{}`)
		setCacheOrigin(t, repo, meta.URL())
	}

	for _, host := range []string{"github.com", "gitlab.com"} {
		meta := versioning.DependencyMeta{Site: host, User: "fixture", Repo: "shared"}
		repo, err := pcx.EnsureDependencyCachedContext(t.Context(), meta, false)
		require.NoError(t, err)
		origin, err := cachedRepositoryOrigin(repo)
		require.NoError(t, err)
		require.Equal(t, meta.URL(), origin)
	}
}

func TestRepositoryCacheRejectsMismatchedOrigin(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{Site: "gitlab.com", User: "fixture", Repo: "shared"}
	path := meta.CachePath(cacheDir)
	repo := cacheFixtureRepository(t, path, `{}`)
	setCacheOrigin(t, repo, "https://github.com/fixture/shared")
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}

	_, err := pcx.EnsureDependencyCachedContext(t.Context(), meta, false)
	require.ErrorIs(t, err, os.ErrInvalid)
	_, err = pcx.packageFromCachedRevisionContext(t.Context(), meta, path)
	require.ErrorIs(t, err, os.ErrInvalid)
	_, err = pcx.latestTagFromCacheContext(t.Context(), meta)
	require.ErrorIs(t, err, os.ErrInvalid)
	_, err = pcx.cachedRepoHasNoOriginContext(t.Context(), meta)
	require.ErrorIs(t, err, os.ErrInvalid)
	destination := filepath.Join(t.TempDir(), "dependency")
	_, err = pcx.EnsureDependencyFromCacheContext(t.Context(), meta, destination, false)
	require.ErrorIs(t, err, os.ErrInvalid)
	_, err = os.Lstat(destination)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(path, "pawn.json"))
	require.NoError(t, err)
}

func TestLegacyRepositoryCacheMigratesOnlyMatchingOrigin(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{"git@github.com:fixture/shared.git", "https://gitlab.com/fixture/shared", ""} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()

			cacheDir := t.TempDir()
			meta := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "shared"}
			legacy := filepath.Join(cacheDir, "packages", meta.User, meta.Repo, "default")
			repo := cacheFixtureRepository(t, legacy, `{}`)
			if origin != "" {
				setCacheOrigin(t, repo, origin)
			}

			store := &fakeRepositoryStore{
				openFn: git.PlainOpen,
				cloneFn: func(path string, _ bool, options *git.CloneOptions) (*git.Repository, error) {
					require.Equal(t, meta.URL(), options.URL)
					repo := cacheFixtureRepository(t, path, `{}`)
					setCacheOrigin(t, repo, options.URL)
					return repo, nil
				},
			}
			pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir, RepoStore: store}}

			_, err := pcx.EnsureDependencyCachedContext(t.Context(), meta, false)
			require.NoError(t, err)
			require.FileExists(t, filepath.Join(meta.CachePath(cacheDir), "pawn.json"))
			if origin == "git@github.com:fixture/shared.git" {
				require.Zero(t, store.cloneCalls)
				_, err := os.Lstat(legacy)
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.Equal(t, 1, store.cloneCalls)
				require.FileExists(t, filepath.Join(legacy, "pawn.json"))
			}
		})
	}
}

func TestRepositoryOriginIdentityAllowsTransportChanges(t *testing.T) {
	t.Parallel()

	meta := versioning.DependencyMeta{Site: "github.com", User: "Fixture", Repo: "shared"}
	for _, origin := range []string{"https://github.com/fixture/shared.git", "git@github.com:Fixture/shared", "ssh://git@github.com:22/Fixture/shared"} {
		require.True(t, repositoryOriginMatches(origin, meta), origin)
	}
	for _, origin := range []string{"https://gitlab.com/Fixture/shared", "https://github.com/other/shared", "https://github.com:8443/Fixture/shared"} {
		require.False(t, repositoryOriginMatches(origin, meta), origin)
	}
	meta.Site = "github.com:443"
	require.True(t, repositoryOriginMatches("https://github.com/fixture/shared", meta))
	meta.Site = "[::1]:8443"
	require.True(t, repositoryOriginMatches("https://[::1]:8443/Fixture/shared", meta))
	meta = versioning.DependencyMeta{Site: "github.com", User: "Fixture", Repo: "shared.git"}
	require.True(t, repositoryOriginMatches("https://github.com/fixture/shared.git", meta))
}

func TestLegacyRepositoryCacheRejectsSymlink(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "shared"}
	external := filepath.Join(t.TempDir(), "repository")
	repo := cacheFixtureRepository(t, external, `{}`)
	setCacheOrigin(t, repo, meta.URL())
	legacy := filepath.Join(cacheDir, "packages", meta.User, meta.Repo, "default")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacy), 0o700))
	if err := os.Symlink(external, legacy); errors.Is(err, os.ErrPermission) {
		t.Skipf("symlinks unavailable: %v", err)
	} else {
		require.NoError(t, err)
	}
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}

	_, err := pcx.EnsureDependencyCachedContext(t.Context(), meta, false)
	require.ErrorIs(t, err, os.ErrInvalid)
	require.FileExists(t, filepath.Join(external, "pawn.json"))
	info, err := os.Lstat(legacy)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestRepositoryCacheCancellationPrecedesMigration(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := pcx.EnsureDependencyCachedContext(ctx, versioning.DependencyMeta{User: "fixture", Repo: "shared"}, false)
	require.True(t, errors.Is(err, context.Canceled))
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}
