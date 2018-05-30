package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestEnsurePlugins(t *testing.T) {
	type args struct {
		cfg types.Runtime
	}
	tests := []struct {
		name        string
		args        args
		wantFiles   []string
		wantPlugins []types.Plugin
		wantErr     bool
	}{
		{"streamer-linux", args{
			types.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "samp-incognito", Repo: "samp-streamer-plugin", Tag: "v2.9.2"}},
			}}, []string{"plugins/streamer.so"}, []types.Plugin{"streamer"}, false},
		{"streamer-windows", args{
			types.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "samp-incognito", Repo: "samp-streamer-plugin", Tag: "v2.9.2"}},
			}}, []string{"plugins/streamer.dll"}, []types.Plugin{"streamer"}, false},
		{"mysql-linux", args{
			types.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "pBlueG", Repo: "SA-MP-MySQL", Tag: "R41-4"}},
			}}, []string{"plugins/mysql.so"}, []types.Plugin{"mysql"}, false},
		{"mysql-windows", args{
			types.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "pBlueG", Repo: "SA-MP-MySQL", Tag: "R41-4"}},
			}}, []string{"plugins/mysql.dll"}, []types.Plugin{"mysql"}, false},
		{"bitmapper-linux", args{
			types.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "Southclaws", Repo: "samp-bitmapper", Tag: "0.2.1"}},
			}}, []string{"plugins/bitmapper.so"}, []types.Plugin{"bitmapper"}, false},
		{"bitmapper-windows", args{
			types.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "Southclaws", Repo: "samp-bitmapper", Tag: "0.2.1"}},
			}}, []string{"plugins/bitmapper.dll"}, []types.Plugin{"bitmapper"}, false},
		{"PawnPlus-linux", args{
			types.Runtime{
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{{User: "IllidanS4", Repo: "PawnPlus", Tag: "v0.5"}},
			}}, []string{"plugins/PawnPlus.so"}, []types.Plugin{"PawnPlus"}, false},
		{"PawnPlus-windows", args{
			types.Runtime{
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{{User: "IllidanS4", Repo: "PawnPlus", Tag: "v0.5"}},
			}}, []string{"plugins/PawnPlus.dll"}, []types.Plugin{"PawnPlus"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.cfg.WorkingDir = filepath.Join("./tests/ensure", tt.name)
			os.MkdirAll(tt.args.cfg.WorkingDir, 0755)

			t.Log("First call to Ensure - from internet")
			err := EnsurePlugins(context.Background(), gh, &tt.args.cfg, "./tests/cache", true)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// the first call to EnsurePlugins modifies this list, we don't want duplicates in the next test, so clear it
			tt.args.cfg.Plugins = []types.Plugin{}

			t.Log("Second call to Ensure - from cache")
			err = EnsurePlugins(context.Background(), gh, &tt.args.cfg, "./tests/cache", false)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			for _, file := range tt.wantFiles {
				assert.True(t, util.Exists(filepath.Join("./tests/ensure", tt.name, file)))
			}

			assert.Equal(t, tt.wantPlugins, tt.args.cfg.Plugins)
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
		wantPkg types.Package
		wantErr bool
	}{
		{"streamer", args{versioning.DependencyMeta{Site: "github.com", User: "samp-incognito", Repo: "samp-streamer-plugin"}}, types.Package{
			DependencyMeta: versioning.DependencyMeta{
				User: "samp-incognito",
				Repo: "samp-streamer-plugin",
			},
			Resources: []types.Resource{
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
			Runtime: &types.Runtime{
				Plugins: []types.Plugin{
					"samp-incognito/samp-streamer-plugin",
				},
			},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := types.GetRemotePackage(context.Background(), gh, tt.args.meta)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantPkg, gotPkg)
		})
	}
}
