package pkgcontext

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func TestTargetedCacheGraphRefreshRecognizesDirectDependencyRegardlessOfOrder(t *testing.T) {
	for _, directFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "transitive first", true: "direct first"}[directFirst], func(t *testing.T) {
			pcx := lifecycleContext(t, "json")
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/parent:1.0.0", "fixture/shared:latest"}
			if directFirst {
				pcx.Package.Dependencies[0], pcx.Package.Dependencies[1] = pcx.Package.Dependencies[1], pcx.Package.Dependencies[0]
			}
			require.NoError(t, pcx.Package.WriteDefinition())
			seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: "parent"}).CachePath(pcx.CacheDir), `{"dependencies":["fixture/shared:latest"]}`)
			for _, name := range []string{"child-old", "child-new"} {
				seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: name}).CachePath(pcx.CacheDir), `{}`)
			}
			shared, err := versioning.DependencyString("fixture/shared:latest").Explode()
			require.NoError(t, err)
			seedStaleCachedLatestDependency(t, pcx.CacheDir, shared,
				`{"dependencies":["fixture/child-old:1.0.0"]}`,
				`{"dependencies":["fixture/child-new:1.0.0"]}`,
			)
			repo, err := pcx.repositoryStore().Open(shared.CachePath(pcx.CacheDir))
			require.NoError(t, err)
			require.NoError(t, repo.DeleteRemote("origin"))
			first := shared
			first.Tag = "1.0.0"
			ref, err := versioning.RefFromTag(repo, first)
			require.NoError(t, err)
			require.NoError(t, pcx.InitLockfileResolver("test"))
			require.NoError(t, pcx.RecordDependencyResolution(shared, lockfile.DependencyResolution{Commit: ref.Hash().String(), Resolved: "1.0.0"}, false, ""))
			target := shared
			target.Tag = ""
			err = pcx.refreshDependencyGraphContext(context.Background(), DependencyUpdateRequest{
				Enabled: true, Target: "fixture/shared", TargetMeta: target,
			})
			require.NoError(t, err)
			var names []string
			for _, dep := range pcx.AllDependencies {
				names = append(names, dep.Repo)
			}
			assert.Contains(t, names, "child-new")
			assert.NotContains(t, names, "child-old")
		})
	}
}

func TestEnsureProjectRequiresPublishedLatestTagInCache(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/shared:latest"}
	require.NoError(t, pcx.Package.WriteDefinition())
	meta, err := pcx.Package.Dependencies[0].Explode()
	require.NoError(t, err)
	cacheFixtureRepository(t, meta.CachePath(pcx.CacheDir), "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/shared/releases" {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`[{"tag_name":"stable","draft":false,"prerelease":false}]`))
			assert.NoError(t, err)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	pcx.GitHub = github.NewClient(server.Client())
	pcx.GitHub.BaseURL, err = url.Parse(server.URL + "/")
	require.NoError(t, err)
	manifest := filepath.Join(pcx.Package.LocalPath, "pawn.json")
	before, err := os.ReadFile(manifest)
	require.NoError(t, err)
	_, err = pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stable")
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.NoDirExists(t, filepath.Join(pcx.Package.Vendor, meta.Repo))
}
