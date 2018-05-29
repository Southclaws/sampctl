package runtime

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

func TestNewConfigFromEnvironment(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		env     map[string]string
		args    args
		genCfg  types.Runtime
		wantCfg types.Runtime
		wantErr bool
	}{
		{
			"minimal",
			map[string]string{"SAMP_RCON_PASSWORD": "changed"},
			args{"./tests/from-env"},
			types.Runtime{
				WorkingDir: "./tests/from-env",
				Version:    "0.3.7",
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				Plugins: []types.Plugin{
					"streamer",
					"zeex/samp-plugin-crashdetect",
				},
				Port:       &[]int{8080}[0],
				Hostname:   &[]string{"Test"}[0],
				MaxPlayers: &[]int{32}[0],
				Language:   &[]string{"English"}[0],
				Announce:   &[]bool{true}[0],
				RCON:       &[]bool{true}[0],
			},
			types.Runtime{
				WorkingDir: "./tests/from-env",
				Platform:   runtime.GOOS,
				PluginDeps: []versioning.DependencyMeta{
					{Site: "github.com", User: "zeex", Repo: "samp-plugin-crashdetect"},
				},
				Format:  "json",
				Version: "0.3.7",
				Mode:    types.Server,
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				Plugins: []types.Plugin{
					"streamer",
				},
				RCONPassword: &[]string{"changed"}[0],
				Port:         &[]int{8080}[0],
				Hostname:     &[]string{"Test"}[0],
				MaxPlayers:   &[]int{32}[0],
				Language:     &[]string{"English"}[0],
				Announce:     &[]bool{true}[0],
				RCON:         &[]bool{true}[0],
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.genCfg.ToJSON()
			assert.NoError(t, err)

			for k, v := range tt.env {
				os.Setenv(k, v) // nolint
			}
			gotCfg, err := NewConfigFromEnvironment(tt.args.dir)

			// NewConfigFromEnvironment uses runtime.GOOS so the comparison should to
			tt.wantCfg.Platform = runtime.GOOS

			assert.NoError(t, err)
			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestEnsureScripts(t *testing.T) {
	tests := []struct {
		name     string
		config   types.Runtime
		wantErrs bool
	}{
		{
			"minimal",
			types.Runtime{
				WorkingDir: "./tests/validate",
				Gamemodes: []string{
					"rivershell",
				},
				RCONPassword: &[]string{"changed"}[0],
				Port:         &[]int{8080}[0],
				Hostname:     &[]string{"Test"}[0],
				MaxPlayers:   &[]int{32}[0],
				Language:     &[]string{"English"}[0],
				Announce:     &[]bool{true}[0],
				RCON:         &[]bool{true}[0],
			},
			false,
		},
		{
			"minimal_fail",
			types.Runtime{
				WorkingDir: "./tests/validate",
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				RCONPassword: &[]string{"changed"}[0],
				Port:         &[]int{8080}[0],
				Hostname:     &[]string{"Test"}[0],
				MaxPlayers:   &[]int{32}[0],
				Language:     &[]string{"English"}[0],
				Announce:     &[]bool{true}[0],
				RCON:         &[]bool{true}[0],
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsureScripts(tt.config)
			if tt.wantErrs {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
