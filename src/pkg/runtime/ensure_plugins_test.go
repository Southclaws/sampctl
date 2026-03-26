package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	res "github.com/Southclaws/sampctl/src/pkg/package/resource"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsurePlugins(t *testing.T) {
	tests := []struct {
		name           string
		cfg            run.Runtime
		meta           versioning.DependencyMeta
		archiveName    string
		archiveFiles   map[string]string
		resources      []res.Resource
		wantFiles      []string
		wantPlugins    []run.Plugin
		wantComponents []run.Plugin
	}{
		{
			name:        "plugin-linux",
			cfg:         run.Runtime{Platform: "linux"},
			meta:        versioning.DependencyMeta{User: "fixture", Repo: "streamer", Tag: "v1.0.0"},
			archiveName: "streamer-v1.0.0.tar.gz",
			archiveFiles: map[string]string{
				"pawno/include/streamer.inc": "#define STREAMER 1\n",
				"plugins/streamer.so":        "fixture",
			},
			resources: []res.Resource{{
				Name:     `^streamer-v1\.0\.0\.tar\.gz$`,
				Platform: "linux",
				Archive:  true,
				Includes: []string{"pawno/include/streamer.inc"},
				Plugins:  []string{"plugins/streamer.so"},
			}},
			wantFiles:   []string{"plugins/streamer.so"},
			wantPlugins: []run.Plugin{"streamer"},
		},
		{
			name:        "plugin-windows",
			cfg:         run.Runtime{Platform: "windows"},
			meta:        versioning.DependencyMeta{User: "fixture", Repo: "mysql", Tag: "v1.0.0"},
			archiveName: "mysql-v1.0.0.zip",
			archiveFiles: map[string]string{
				"plugins/mysql.dll": "fixture",
			},
			resources: []res.Resource{{
				Name:     `^mysql-v1\.0\.0\.zip$`,
				Platform: "windows",
				Archive:  true,
				Plugins:  []string{"plugins/mysql.dll"},
			}},
			wantFiles:   []string{"plugins/mysql.dll"},
			wantPlugins: []run.Plugin{"mysql"},
		},
		{
			name:        "component-openmp-linux",
			cfg:         run.Runtime{Platform: "linux", Version: "v1.0.0-openmp", RuntimeType: run.RuntimeTypeOpenMP},
			meta:        versioning.DependencyMeta{Scheme: "component", User: "fixture", Repo: "pawnraknet", Tag: "v1.0.0"},
			archiveName: "pawnraknet-v1.0.0.tar.gz",
			archiveFiles: map[string]string{
				"plugins/pawnraknet.so": "fixture",
			},
			resources: []res.Resource{{
				Name:     `^pawnraknet-v1\.0\.0\.tar\.gz$`,
				Platform: "linux",
				Archive:  true,
				Plugins:  []string{"plugins/pawnraknet.so"},
			}},
			wantFiles:      []string{"components/pawnraknet.so"},
			wantComponents: []run.Plugin{"pawnraknet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := filepath.Join(t.TempDir(), "cache")
			workingDir := filepath.Join(t.TempDir(), "work")
			tt.cfg.WorkingDir = workingDir
			tt.cfg.PluginDeps = []versioning.DependencyMeta{tt.meta}

			seedCachedPluginPackage(t, cacheDir, tt.meta, pluginFixturePackage(tt.meta, tt.resources), tt.archiveName, tt.archiveFiles)

			err := EnsurePlugins(EnsurePluginsRequest{
				Context:  context.Background(),
				GitHub:   nil,
				Config:   &tt.cfg,
				CacheDir: cacheDir,
				NoCache:  false,
			})
			assert.NoError(t, err)

			tt.cfg.Plugins = nil
			tt.cfg.Components = nil

			err = EnsurePlugins(EnsurePluginsRequest{
				Context:  context.Background(),
				GitHub:   nil,
				Config:   &tt.cfg,
				CacheDir: cacheDir,
				NoCache:  false,
			})
			assert.NoError(t, err)

			for _, file := range tt.wantFiles {
				assert.True(t, fs.Exists(filepath.Join(workingDir, file)))
			}

			assert.Equal(t, tt.wantPlugins, tt.cfg.Plugins)
			assert.Equal(t, tt.wantComponents, tt.cfg.Components)
		})
	}
}

func TestGetResourceAndPath(t *testing.T) {
	t.Parallel()

	resource, err := GetResource([]res.Resource{
		{Name: `linux-default`, Platform: "linux", Archive: true, Plugins: []string{"plugins/test.so"}},
		{Name: `linux-openmp`, Platform: "linux", Version: "v1.0.0-openmp", Archive: true, Plugins: []string{"plugins/test.so"}},
	}, "linux", "v1.0.0-openmp")
	require.NoError(t, err)
	assert.Equal(t, "linux-openmp", resource.Name)

	resource, err = GetResource([]res.Resource{{Name: `linux-default`, Platform: "linux", Archive: true, Plugins: []string{"plugins/test.so"}}}, "linux", "0.3.7")
	require.NoError(t, err)
	assert.Equal(t, "linux-default", resource.Name)

	_, err = GetResource([]res.Resource{{Name: `linux-default`, Platform: "linux"}}, "windows", "0.3.7")
	require.Error(t, err)

	assert.Equal(t, filepath.Join("plugins", "streamer", "latest"), GetResourcePath(versioning.DependencyMeta{Repo: "streamer"}))
	assert.Equal(t, filepath.Join("plugins", "streamer", "v1.0.0"), GetResourcePath(versioning.DependencyMeta{Repo: "streamer", Tag: "v1.0.0"}))
}

func TestHasExplicitDependencyReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta versioning.DependencyMeta
		want bool
	}{
		{
			name: "no reference",
			meta: versioning.DependencyMeta{User: "fixture", Repo: "streamer"},
			want: false,
		},
		{
			name: "tag reference",
			meta: versioning.DependencyMeta{User: "fixture", Repo: "streamer", Tag: "v1.0.0"},
			want: true,
		},
		{
			name: "branch reference",
			meta: versioning.DependencyMeta{User: "fixture", Repo: "streamer", Branch: "main"},
			want: true,
		},
		{
			name: "commit reference",
			meta: versioning.DependencyMeta{User: "fixture", Repo: "streamer", Commit: "abc123"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hasExplicitDependencyReference(tt.meta))
		})
	}
}

func TestPluginFromCacheLatestUsesResolvedTagAsset(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "streamer", Tag: "latest"}
	resourceDef := res.Resource{
		Name:     `^streamer-v1\.2\.3\.tar\.gz$`,
		Platform: "linux",
		Archive:  true,
		Plugins:  []string{"plugins/streamer.so"},
	}

	seedCachedPluginPackage(t, cacheDir, meta, pluginFixturePackage(meta, []res.Resource{resourceDef}), "placeholder.txt", map[string]string{"placeholder.txt": "fixture"})

	assetPath := filepath.Join(t.TempDir(), "streamer-v1.2.3.tar.gz")
	require.NoError(t, os.WriteFile(assetPath, []byte("fixture"), 0o644))

	matcher := regexp.MustCompile(resourceDef.Name)
	ghr := infraresource.NewGitHubReleaseResource(meta, matcher, infraresource.ResourceTypePlugin, nil)
	ghr.SetCacheDir(cacheDir)
	ghr.SetCacheTTL(0)
	ghr.SetLocalPath(assetPath)
	require.NoError(t, ghr.EnsureFromLocal(context.Background(), "v1.2.3", ""))
	_, expectedPath := ghr.Cached("v1.2.3")

	hit, filename, resource, err := PluginFromCache(meta, "linux", "0.3.7", cacheDir)
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, expectedPath, filename)
	require.NotNil(t, resource)
	assert.Equal(t, resourceDef.Name, resource.Name)
}

func TestDetectArchiveExt(t *testing.T) {
	t.Parallel()

	zipPath := filepath.Join(t.TempDir(), "fixture.zip")
	createRuntimeZipArchive(t, zipPath, map[string]string{"plugins/test.dll": "fixture"})
	assert.Equal(t, ".zip", detectArchiveExt(zipPath))

	gzPath := filepath.Join(t.TempDir(), "fixture.tar.gz")
	createRuntimeTgzArchive(t, gzPath, map[string]string{"plugins/test.so": "fixture"})
	assert.Equal(t, ".gz", detectArchiveExt(gzPath))

	plainPath := filepath.Join(t.TempDir(), "plain.bin")
	require.NoError(t, os.WriteFile(plainPath, []byte("text"), 0o644))
	assert.Empty(t, detectArchiveExt(plainPath))

	shortPath := filepath.Join(t.TempDir(), "short.bin")
	require.NoError(t, os.WriteFile(shortPath, []byte{0x1f}, 0o644))
	assert.Empty(t, detectArchiveExt(shortPath))

	assert.Empty(t, detectArchiveExt(filepath.Join(t.TempDir(), "missing.bin")))
}

func TestCachedPackageResourceAsset(t *testing.T) {
	t.Parallel()

	cachePath := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.MkdirAll(filepath.Join(cachePath, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cachePath, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, ".git", "ignored.zip"), []byte("fixture"), 0o644))
	want := filepath.Join(cachePath, "assets", "streamer.zip")
	require.NoError(t, os.WriteFile(want, []byte("fixture"), 0o644))

	got, hit := cachedPackageResourceAsset(cachePath, regexp.MustCompile(`\.zip$`))
	assert.True(t, hit)
	assert.Equal(t, want, got)

	got, hit = cachedPackageResourceAsset(cachePath, nil)
	assert.False(t, hit)
	assert.Empty(t, got)

	got, hit = cachedPackageResourceAsset(filepath.Join(t.TempDir(), "missing"), regexp.MustCompile(`\.zip$`))
	assert.False(t, hit)
	assert.Empty(t, got)
}

func TestEnsureVersionedPluginCachedUsesLocalPackageAsset(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "streamer", Tag: "v1.0.0"}
	resourceDef := res.Resource{
		Name:     `^streamer\.so$`,
		Platform: "linux",
		Plugins:  []string{"streamer.so"},
	}
	assetPath := seedCachedSinglePluginAsset(t, cacheDir, meta, resourceDef, "streamer.so")

	filename, resource, err := EnsureVersionedPluginCached(EnsureVersionedPluginCachedRequest{
		Context:  context.Background(),
		Meta:     meta,
		Platform: "linux",
		Version:  "0.3.7",
		CacheDir: cacheDir,
	})
	require.NoError(t, err)
	require.NotNil(t, resource)
	assert.Equal(t, assetPath, filename)
	assert.Equal(t, resourceDef.Name, resource.Name)
}

func TestEnsureVersionedPluginSingleFile(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	workDir := filepath.Join(t.TempDir(), "work")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "streamer", Tag: "v1.0.0"}
	resourceDef := res.Resource{
		Name:     `^streamer\.so$`,
		Platform: "linux",
		Plugins:  []string{"streamer.so"},
	}
	seedCachedSinglePluginAsset(t, cacheDir, meta, resourceDef, "streamer.so")

	files, err := EnsureVersionedPlugin(EnsureVersionedPluginRequest{
		Context:       context.Background(),
		Meta:          meta,
		Dir:           workDir,
		Platform:      "linux",
		Version:       "0.3.7",
		CacheDir:      cacheDir,
		PluginDestDir: "plugins",
		Plugins:       true,
	})
	require.NoError(t, err)
	assert.Equal(t, []run.Plugin{"streamer.so"}, files)
	assert.True(t, fs.Exists(filepath.Join(workDir, "plugins", "streamer.so")))
}

func TestEnsureVersionedPluginSingleFileRequiresDestination(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "streamer", Tag: "v1.0.0"}
	resourceDef := res.Resource{
		Name:     `^streamer\.so$`,
		Platform: "linux",
		Plugins:  []string{"streamer.so"},
	}
	seedCachedSinglePluginAsset(t, cacheDir, meta, resourceDef, "streamer.so")

	_, err := EnsureVersionedPlugin(EnsureVersionedPluginRequest{
		Context:  context.Background(),
		Meta:     meta,
		Dir:      filepath.Join(t.TempDir(), "work"),
		Platform: "linux",
		Version:  "0.3.7",
		CacheDir: cacheDir,
		Plugins:  true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pluginDestDir is required")
}

func seedCachedSinglePluginAsset(t *testing.T, cacheDir string, meta versioning.DependencyMeta, resourceDef res.Resource, filename string) string {
	t.Helper()

	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o700))

	pkg := pluginFixturePackage(meta, []res.Resource{resourceDef})
	data, err := json.Marshal(pkg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), data, 0o644))

	assetPath := filepath.Join(cachePath, filename)
	require.NoError(t, os.WriteFile(assetPath, []byte("fixture"), 0o644))

	return assetPath
}
