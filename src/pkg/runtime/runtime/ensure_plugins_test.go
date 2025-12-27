package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	"github.com/Southclaws/sampctl/src/resource"
)

func TestEnsurePlugins(t *testing.T) {
	type args struct {
		cfg run.Runtime
	}
	tests := []struct {
		name           string
		args           args
		wantFiles      []string
		wantPlugins    []run.Plugin
		wantComponents []run.Plugin
		wantErr        bool
	}{
		{"streamer-linux", args{
			run.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "samp-incognito", Repo: "samp-streamer-plugin", Tag: "v2.9.2"}},
			},
		}, []string{"plugins/streamer.so"}, []run.Plugin{"streamer"}, nil, false},
		{"streamer-linux-openmp", args{
			run.Runtime{
				Platform:    "linux",
				Version:     "v1.0.0-openmp",
				RuntimeType: run.RuntimeTypeOpenMP,
				PluginDeps:  []versioning.DependencyMeta{{User: "samp-incognito", Repo: "samp-streamer-plugin", Tag: "v2.9.2"}},
			},
		}, []string{"plugins/streamer.so"}, []run.Plugin{"streamer"}, nil, false},
		{"streamer-windows", args{
			run.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "samp-incognito", Repo: "samp-streamer-plugin", Tag: "v2.9.2"}},
			},
		}, []string{"plugins/streamer.dll"}, []run.Plugin{"streamer"}, nil, false},
		{"mysql-linux", args{
			run.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "pBlueG", Repo: "SA-MP-MySQL", Tag: "R41-4"}},
			},
		}, []string{"plugins/mysql.so"}, []run.Plugin{"mysql"}, nil, false},
		{"mysql-windows", args{
			run.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "pBlueG", Repo: "SA-MP-MySQL", Tag: "R41-4"}},
			},
		}, []string{"plugins/mysql.dll"}, []run.Plugin{"mysql"}, nil, false},
		{"bitmapper-linux", args{
			run.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "Southclaws", Repo: "samp-bitmapper", Tag: "0.2.1"}},
			},
		}, []string{"plugins/bitmapper.so"}, []run.Plugin{"bitmapper"}, nil, false},
		{"bitmapper-windows", args{
			run.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "Southclaws", Repo: "samp-bitmapper", Tag: "0.2.1"}},
			},
		}, []string{"plugins/bitmapper.dll"}, []run.Plugin{"bitmapper"}, nil, false},
		{"PawnPlus-linux", args{
			run.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "IllidanS4", Repo: "PawnPlus", Tag: "v0.5"}},
			},
		}, []string{"plugins/PawnPlus.so"}, []run.Plugin{"PawnPlus"}, nil, false},
		{"PawnPlus-windows", args{
			run.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "IllidanS4", Repo: "PawnPlus", Tag: "v0.5"}},
			},
		}, []string{"plugins/PawnPlus.dll"}, []run.Plugin{"PawnPlus"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.cfg.WorkingDir = filepath.Join("./tests/ensure", tt.name)
			_ = os.MkdirAll(tt.args.cfg.WorkingDir, 0o700)

			t.Log("First call to Ensure - from internet")
			err := EnsurePlugins(context.Background(), gh, &tt.args.cfg, "./tests/cache", true)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// the first call to EnsurePlugins modifies this list, we don't want duplicates in the next test, so clear it
			tt.args.cfg.Plugins = []run.Plugin{}

			t.Log("Second call to Ensure - from cache")
			err = EnsurePlugins(context.Background(), gh, &tt.args.cfg, "./tests/cache", false)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			for _, file := range tt.wantFiles {
				assert.True(t, fs.Exists(filepath.Join("./tests/ensure", tt.name, file)))
			}

			assert.Equal(t, tt.wantPlugins, tt.args.cfg.Plugins)
			assert.Equal(t, tt.wantComponents, tt.args.cfg.Components)
		})
	}
}

func TestGetPluginRemotePackage(t *testing.T) {
	type args struct {
		meta versioning.DependencyMeta
	}
	tests := []struct {
		name    string
		args    args
		wantPkg pawnpackage.Package
		wantErr bool
	}{
		{"streamer", args{versioning.DependencyMeta{Site: "github.com", User: "samp-incognito", Repo: "samp-streamer-plugin"}}, pawnpackage.Package{
			DependencyMeta: versioning.DependencyMeta{
				User: "samp-incognito",
				Repo: "samp-streamer-plugin",
			},
			Resources: []resource.Resource{
				{
					Name:     "^samp-streamer-plugin-(.*).zip$",
					Platform: "linux",
					Archive:  true,
					Includes: []string{"pawno/include"},
					Plugins:  []string{"plugins/streamer.so"},
				},
				{
					Name:     "^samp-streamer-plugin-(.*).zip$",
					Platform: "windows",
					Archive:  true,
					Includes: []string{"pawno/include"},
					Plugins:  []string{"plugins/streamer.dll"},
				},
			},
			Runtime: &run.Runtime{
				Plugins: []run.Plugin{
					"samp-incognito/samp-streamer-plugin",
				},
			},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := pawnpackage.GetRemotePackage(context.Background(), gh, tt.args.meta)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantPkg, gotPkg)
		})
	}
}
