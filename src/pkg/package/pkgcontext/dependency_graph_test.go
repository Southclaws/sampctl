package pkgcontext

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/resource"
)

func TestDependencyGraphSelectedRevisionFailureKeepsPartialState(t *testing.T) {
	for _, stage := range []string{"cache", "read package metadata"} {
		t.Run(stage, func(t *testing.T) {
			pcx := lifecycleContext(t, "json")
			pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/before:1.0.0", "fixture/broken@release", "fixture/after:1.0.0"}
			before := graphRevisionMeta(t, pcx.Package.Dependencies[0])
			broken := graphRevisionMeta(t, pcx.Package.Dependencies[1])
			after := graphRevisionMeta(t, pcx.Package.Dependencies[2])
			for _, meta := range []versioning.DependencyMeta{before, broken, after} {
				seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), graphRevisionDefinition(t, meta.Repo))
			}
			selectedCommit := strings.Repeat("f", 40)
			lockGraphRevision(t, pcx, broken, selectedCommit)
			pcx.RepoStore = &fakeRepositoryStore{
				openFn: func(path string) (*git.Repository, error) {
					if stage == "cache" && path == broken.CachePath(pcx.CacheDir) {
						return nil, os.ErrPermission
					}
					return git.PlainOpen(path)
				},
				cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
					t.Fatal("seeded graph must not clone repositories")
					return nil, nil
				},
			}

			err := pcx.EnsureDependenciesCachedContext(t.Context())
			var dependencyError *DependencyError
			require.ErrorAs(t, err, &dependencyError)
			assert.Equal(t, broken.String(), dependencyError.Dependency)
			assert.Equal(t, stage, dependencyError.Operation)
			if stage == "cache" {
				assert.ErrorIs(t, err, os.ErrPermission)
			} else {
				assert.Contains(t, err.Error(), selectedCommit)
			}
			assert.Equal(t, []versioning.DependencyMeta{before, after}, pcx.AllDependencies)
			assert.Equal(t, []versioning.DependencyMeta{before, after}, pcx.AllPlugins)
			assert.Equal(t, graphRevisionIncludePaths(pcx.Package.Vendor, "before", "after"), pcx.AllIncludePaths)
		})
	}
}

func TestDependencyGraphLockedRevisionKeepsDepthFirstOrder(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")
	pcx.Package.Dependencies = []versioning.DependencyString{
		"fixture/selected@release", "fixture/after:1.0.0", "fixture/selected@release", "fixture/child:1.0.0",
	}
	selected := graphRevisionMeta(t, pcx.Package.Dependencies[0])
	after := graphRevisionMeta(t, pcx.Package.Dependencies[1])
	child := graphRevisionMeta(t, pcx.Package.Dependencies[3])
	cachePath := selected.CachePath(pcx.CacheDir)
	repo := seedReliabilityRepo(t, cachePath, graphRevisionDefinition(t, "selected", "fixture/child:1.0.0", "fixture/child:1.0.0"))
	head, err := repo.Head()
	require.NoError(t, err)
	seedReliabilityRepo(t, child.CachePath(pcx.CacheDir), graphRevisionDefinition(t, "child", "fixture/selected@release"))
	seedReliabilityRepo(t, after.CachePath(pcx.CacheDir), graphRevisionDefinition(t, "after"))
	lockGraphRevision(t, pcx, selected, head.Hash().String())
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), []byte(graphRevisionDefinition(t, "working-tree", "fixture/not-selected:1.0.0")), 0o600))
	pcx.RepoStore = &fakeRepositoryStore{
		openFn: git.PlainOpen,
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
			t.Fatal("selected revision, cycles, and duplicates must use seeded repositories")
			return nil, nil
		},
	}

	require.NoError(t, pcx.EnsureDependenciesCachedContext(t.Context()))
	assert.Equal(t, []versioning.DependencyMeta{selected, child, after}, pcx.AllDependencies)
	assert.Equal(t, []versioning.DependencyMeta{selected, child, after}, pcx.AllPlugins)
	assert.Equal(t, graphRevisionIncludePaths(pcx.Package.Vendor, "selected", "child", "after"), pcx.AllIncludePaths)
}

func TestDependencyGraphCancellationAfterMetadataReadKeepsLoadedState(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/selected@release", "fixture/after:1.0.0"}
	selected := graphRevisionMeta(t, pcx.Package.Dependencies[0])
	after := graphRevisionMeta(t, pcx.Package.Dependencies[1])
	child := graphRevisionMeta(t, "fixture/child:1.0.0")
	cachePath := selected.CachePath(pcx.CacheDir)
	repo := seedReliabilityRepo(t, cachePath, graphRevisionDefinition(t, "selected", "fixture/child:1.0.0"))
	head, err := repo.Head()
	require.NoError(t, err)
	seedReliabilityRepo(t, after.CachePath(pcx.CacheDir), graphRevisionDefinition(t, "after"))
	seedReliabilityRepo(t, child.CachePath(pcx.CacheDir), graphRevisionDefinition(t, "child"))
	lockGraphRevision(t, pcx, selected, head.Hash().String())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	cacheOpened, canceledAtRead := false, false
	var opened []string
	pcx.RepoStore = &fakeRepositoryStore{
		openFn: func(path string) (*git.Repository, error) {
			opened = append(opened, path)
			if path == cachePath {
				// The cache ensure opens first; cancel as the protected metadata read opens.
				if cacheOpened {
					cancel()
					canceledAtRead = true
				}
				cacheOpened = true
			}
			return git.PlainOpen(path)
		},
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
			t.Fatal("canceled graph must not clone repositories")
			return nil, nil
		},
	}

	err = pcx.EnsureDependenciesCachedContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
	var dependencyError *DependencyError
	require.ErrorAs(t, err, &dependencyError)
	assert.Equal(t, selected.String(), dependencyError.Dependency)
	assert.Equal(t, "cache", dependencyError.Operation)
	assert.True(t, canceledAtRead)
	assert.Equal(t, []versioning.DependencyMeta{selected}, pcx.AllDependencies)
	assert.Equal(t, []versioning.DependencyMeta{selected}, pcx.AllPlugins)
	assert.Equal(t, graphRevisionIncludePaths(pcx.Package.Vendor, "selected"), pcx.AllIncludePaths)
	assert.NotContains(t, opened, child.CachePath(pcx.CacheDir))
	assert.NotContains(t, opened, after.CachePath(pcx.CacheDir))
}

func graphRevisionMeta(t *testing.T, declaration versioning.DependencyString) versioning.DependencyMeta {
	t.Helper()

	meta, err := declaration.Explode()
	require.NoError(t, err)
	return meta
}

func graphRevisionDefinition(t *testing.T, name string, dependencies ...versioning.DependencyString) string {
	t.Helper()

	data, err := json.Marshal(pawnpackage.Package{
		Repo: name, Dependencies: dependencies,
		Resources: []resource.Resource{{Name: name + ".so", Platform: "linux", Includes: []string{"include"}}},
	})
	require.NoError(t, err)
	return string(data)
}

func lockGraphRevision(t *testing.T, pcx *PackageContext, meta versioning.DependencyMeta, commit string) {
	t.Helper()

	require.NoError(t, pcx.InitLockfileResolver("test"))
	require.NoError(t, pcx.RecordDependencyResolution(meta, lockfile.DependencyResolution{Commit: commit}, false, ""))
	locked := pcx.LockedVersion(meta, false)
	require.Equal(t, commit, locked.Commit)
	require.Equal(t, meta.Branch, locked.Branch)
	require.Empty(t, locked.Tag)
}

func graphRevisionIncludePaths(vendor string, names ...string) []string {
	paths := make([]string, 0, len(names))
	for _, name := range names {
		res := resource.Resource{Name: name + ".so"}
		paths = append(paths, filepath.Join(vendor, res.Path(name)))
	}
	return paths
}
