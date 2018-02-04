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
		{"linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/linux",
				Platform:   "linux",
				PluginDeps: []versioning.DependencyMeta{
					{User: "samp-incognito", Repo: "samp-streamer-plugin"},
					{User: "Zeex", Repo: "samp-plugin-crashdetect"},
					{User: "pBlueG", Repo: "SA-MP-MySQL"},
					{User: "ziggi", Repo: "FCNPC"},
					{User: "BigETI", Repo: "pawn-memory"},
					{User: "Southclaws", Repo: "samp-nolog"},
				},
			},
		}, []string{
			"plugins/streamer.so",
			"plugins/crashdetect.so",
			"plugins/mysql.so",
			"plugins/FCNPC.so",
			"plugins/memory.so",
			"plugins/nolog.so",
		}, []types.Plugin{
			"streamer",
			"crashdetect",
			"mysql",
			"FCNPC",
			"memory",
			"nolog",
		}, false},
		{"windows", args{
			types.Runtime{
				WorkingDir: "./tests/ensure/windows",
				Platform:   "windows",
				PluginDeps: []versioning.DependencyMeta{
					{User: "samp-incognito", Repo: "samp-streamer-plugin"},
					{User: "Zeex", Repo: "samp-plugin-crashdetect"},
					{User: "pBlueG", Repo: "SA-MP-MySQL"},
					{User: "ziggi", Repo: "FCNPC"},
					{User: "BigETI", Repo: "pawn-memory"},
					{User: "urShadow", Repo: "Pawn.RakNet"},
				},
			},
		}, []string{
			"plugins/streamer.dll",
			"plugins/crashdetect.dll",
			"plugins/mysql.dll",
			"plugins/FCNPC.dll",
			"plugins/pawn-memory.dll",
			"plugins/pawnraknet.dll",
		}, []types.Plugin{
			"streamer",
			"crashdetect",
			"mysql",
			"FCNPC",
			"pawn-memory",
			"pawnraknet",
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.MkdirAll(tt.args.cfg.WorkingDir, 0755)

			t.Log("First call to Ensure - from internet")
			err := EnsurePlugins(context.Background(), gh, &tt.args.cfg, "./tests/cache", true)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// the first clal to EnsurePlugins modifies this list, we don't want duplicates in the next test, so clear it
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
