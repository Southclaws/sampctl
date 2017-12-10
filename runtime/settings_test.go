package runtime

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewConfigFromEnvironment(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		env     map[string]string
		args    args
		genCfg  Config
		wantCfg Config
		wantErr bool
	}{
		{
			"minimal",
			map[string]string{"SAMP_RCON_PASSWORD": "changed"},
			args{"./tests/from-env"},
			Config{
				Gamemodes: []string{
					"rivershell",
					"baserace",
				},
				Port:       &[]int{8080}[0],
				Hostname:   &[]string{"Test"}[0],
				MaxPlayers: &[]int{32}[0],
				Language:   &[]string{"English"}[0],
				Announce:   &[]bool{true}[0],
				RCON:       &[]bool{true}[0],
			},
			Config{
				dir: &[]string{"./tests/from-env"}[0],
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
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.genCfg.GenerateJSON(tt.args.dir)

			for k, v := range tt.env {
				os.Setenv(k, v) // nolint
			}
			gotCfg, err := NewConfigFromEnvironment(tt.args.dir)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestConfig_ValidateWorkspace(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name     string
		config   Config
		args     args
		wantErrs bool
	}{
		{
			"minimal",
			Config{
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
			args{"./tests/validate"},
			false,
		},
		{
			"minimal_fail",
			Config{
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
			args{"./tests/validate"},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.config.ValidateWorkspace(tt.args.dir)
			if tt.wantErrs {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}
