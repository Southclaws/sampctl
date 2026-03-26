package pkgcontext

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestResolveDynamicDependencyReferenceLatestUsesConcreteTag(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "latest"}
	seedLatestTagRepo(t, cacheDir, meta, []string{"v1.0.0", "v1.1.0"})

	state := PackageLockfileState{lockfileResolver: &fakeDependencyLock{
		hasPrevious: true,
		previous: lockfile.LockedDependency{
			Constraint: ":latest",
			Resolved:   "v1.0.0",
			Commit:     "abcdef0123456789",
		},
	}}
	pctx := &PackageContext{
		PackageServices:      PackageServices{CacheDir: cacheDir},
		PackageLockfileState: state,
	}

	resolved, err := pctx.resolveDynamicDependencyReference(context.Background(), meta, meta, true)
	require.NoError(t, err)
	assert.Equal(t, "v1.1.0", resolved.Tag)
}

func TestResolveDynamicDependencyReferenceLatestForceUpdatePrefersFreshRelease(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "latest"}
	seedLatestTagRepo(t, cacheDir, meta, []string{"v1.0.0"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/dep/releases" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"tag_name":"v1.1.0","draft":false,"prerelease":false}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	gh := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = baseURL

	pctx := &PackageContext{
		PackageServices: PackageServices{CacheDir: cacheDir, GitHub: gh},
		PackageLockfileState: PackageLockfileState{lockfileResolver: &fakeDependencyLock{
			hasPrevious: true,
			previous:    lockfile.LockedDependency{Constraint: ":latest", Resolved: "v1.0.0", Commit: "abcdef0123456789"},
		}},
	}

	resolved, err := pctx.resolveDynamicDependencyReference(context.Background(), meta, meta, true)
	require.NoError(t, err)
	assert.Equal(t, "v1.1.0", resolved.Tag)
}

func TestEnsurePackageLatestForceUpdateUsesResolvedTag(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	workspace := t.TempDir()
	vendorDir := filepath.Join(workspace, "dependencies")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "latest"}
	seedLatestTagRepo(t, cacheDir, meta, []string{"v1.0.0", "v1.1.0"})

	pcx := &PackageContext{
		Package: pawnpackage.Package{
			LocalPath: workspace,
			Vendor:    vendorDir,
			User:      "local",
			Repo:      "local",
		},
		PackageServices: PackageServices{CacheDir: cacheDir},
		PackageLockfileState: PackageLockfileState{lockfileResolver: &fakeDependencyLock{
			hasPrevious: true,
			previous:    lockfile.LockedDependency{Constraint: ":latest", Resolved: "v1.0.0", Commit: "abcdef0123456789"},
		}},
	}

	require.NoError(t, pcx.EnsurePackage(meta, true))

	repo, err := git.PlainOpen(filepath.Join(vendorDir, meta.Repo))
	require.NoError(t, err)
	tag, err := versioning.GetRepoCurrentVersionedTag(repo)
	require.NoError(t, err)
	require.NotNil(t, tag)
	assert.Equal(t, "v1.1.0", tag.Name)
}

func seedLatestTagRepo(t *testing.T, cacheDir string, meta versioning.DependencyMeta, tags []string) {
	t.Helper()

	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	for index, tagName := range tags {
		require.NoError(t, os.WriteFile(filepath.Join(cachePath, "file.txt"), []byte(tagName), 0o644))
		_, err = wt.Add("file.txt")
		require.NoError(t, err)

		hash, err := wt.Commit(tagName, &git.CommitOptions{
			Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(int64(100+index), 0)},
			Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(int64(100+index), 0)},
		})
		require.NoError(t, err)

		_, err = repo.CreateTag(tagName, hash, nil)
		require.NoError(t, err)
	}
}
