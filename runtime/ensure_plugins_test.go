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
				WorkingDir: "./tests/ensure/streamer-linux",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"samp-incognito/samp-streamer-plugin",
				},
			},
		}, []string{"plugins/streamer.so"}, []types.Plugin{"streamer"}, false},
		{"streamer-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/streamer-windows",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"samp-incognito/samp-streamer-plugin",
				},
			},
		}, []string{"plugins/streamer.dll"}, []types.Plugin{"streamer"}, false},
		{"crashdetect-linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/crashdetect-linux",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"Zeex/samp-plugin-crashdetect",
				},
			},
		}, []string{"plugins/crashdetect.so"}, []types.Plugin{"crashdetect"}, false},
		{"crashdetect-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/crashdetect-windows",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"Zeex/samp-plugin-crashdetect",
				},
			},
		}, []string{"plugins/crashdetect.dll"}, []types.Plugin{"crashdetect"}, false},
		{"mysql-linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/mysql-linux",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"pBlueG/SA-MP-MySQL",
				},
			},
		}, []string{"plugins/mysql.so"}, []types.Plugin{"mysql"}, false},
		{"mysql-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/mysql-windows",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"pBlueG/SA-MP-MySQL",
				},
			},
		}, []string{"plugins/mysql.dll"}, []types.Plugin{"mysql"}, false},
		{"fcnpc-linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/fcnpc-linux",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"ziggi/FCNPC",
				},
			},
		}, []string{"plugins/FCNPC.so"}, []types.Plugin{"FCNPC"}, false},
		{"fcnpc-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/fcnpc-windows",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"ziggi/FCNPC",
				},
			},
		}, []string{"plugins/FCNPC.dll"}, []types.Plugin{"FCNPC"}, false},
		{"pawn-memory-linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/pawn-memory-linux",
				Platform:   "linux",
				Plugins: []types.Plugin{
					"BigETI/pawn-memory",
				},
			},
		}, []string{"plugins/memory.so"}, []types.Plugin{"memory"}, false},
		{"pawn-memory-windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/pawn-memory-windows",
				Platform:   "windows",
				Plugins: []types.Plugin{
					"BigETI/pawn-memory",
				},
			},
		}, []string{"plugins/pawn-memory.dll"}, []types.Plugin{"pawn-memory"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.MkdirAll(tt.args.cfg.WorkingDir, 0755)

			t.Log("First call to Ensure - from internet")
			err := EnsurePlugins(&tt.args.cfg, "./tests/cache")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			t.Log("Second call to Ensure - from cache")
			err = EnsurePlugins(&tt.args.cfg, "./tests/cache")
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
