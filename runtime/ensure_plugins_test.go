package runtime

import (
	"os"
	"testing"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
	"github.com/stretchr/testify/assert"
)

func TestEnsureVersionedPlugin(t *testing.T) {
	type args struct {
		cfg  types.Runtime
		meta versioning.DependencyMeta
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"streamer-linux", args{
			types.Runtime{
				WorkingDir: "./tests/ensure",
				Platform:   "linux",
			},
			versioning.DependencyMeta{"samp-incognito", "samp-streamer-plugin", "", ""},
		}, false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.MkdirAll(tt.args.cfg.WorkingDir, 0755)

			if err := EnsureVersionedPlugin(tt.args.cfg, tt.args.meta, "./tests/cache"); (err != nil) != tt.wantErr {
				t.Errorf("EnsureVersionedPlugin() error = %v, wantErr %v", err, tt.wantErr)
			}
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
