package runtime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
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
