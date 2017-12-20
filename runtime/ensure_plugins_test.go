package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestEnsureVersionedPlugin(t *testing.T) {
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
				WorkingDir: "./tests/ensure",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"samp-incognito/samp-streamer-plugin",
				},
			},
		}, []string{"plugins/streamer.so"}, []types.Plugin{"streamer"}, false},
		{"streamer-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"samp-incognito/samp-streamer-plugin",
				},
			},
		}, []string{"plugins/streamer.dll"}, []types.Plugin{"streamer"}, false},
		{"crashdetect-linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"Zeex/samp-plugin-crashdetect",
				},
			},
		}, []string{"plugins/crashdetect.so"}, []types.Plugin{"crashdetect"}, false},
		{"crashdetect-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"Zeex/samp-plugin-crashdetect",
				},
			},
		}, []string{"plugins/crashdetect.dll"}, []types.Plugin{"crashdetect"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.MkdirAll(tt.args.cfg.WorkingDir, 0755)

			err := EnsurePlugins(&tt.args.cfg, "./tests/cache")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			for _, file := range tt.wantFiles {
				assert.True(t, util.Exists(filepath.Join("./tests/ensure", file)))
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
		{"streamer", args{versioning.DependencyMeta{"samp-incognito", "samp-streamer-plugin", "", ""}}, types.Package{
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
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := GetPluginRemotePackage(tt.args.meta)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantPkg, gotPkg)
		})
	}
}
