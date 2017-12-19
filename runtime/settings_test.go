package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
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
			types.Runtime{
				WorkingDir: "./tests/from-env",
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
			err := GenerateJSON(tt.genCfg)
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

func TestConfig_EnsureScripts(t *testing.T) {
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

func TestConfigFromDirectory(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg types.Runtime
		wantErr bool
	}{
		{"both basic", args{"./tests/load-both"}, types.Runtime{
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"both large", args{"./tests/load-yaml"}, types.Runtime{
			Gamemodes: []string{
				"rivershell",
				"baserace",
			},
			RCONPassword: &[]string{"test"}[0],
			Port:         &[]int{8080}[0],
			Hostname:     &[]string{"Test"}[0],
			MaxPlayers:   &[]int{32}[0],
			Language:     &[]string{"English"}[0],
			Announce:     &[]bool{true}[0],
			RCON:         &[]bool{true}[0],
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.args.dir
			tt.wantCfg.WorkingDir = dir

			GenerateJSON(tt.wantCfg)
			GenerateYAML(tt.wantCfg)

			gotCfg, err := ConfigFromDirectory(tt.args.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigFromDirectory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestConfigFromJSON(t *testing.T) {
	type args struct {
		file string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg types.Runtime
		wantErr bool
	}{
		{"json basic", args{"./tests/load-json/samp.json"}, types.Runtime{
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"json large", args{"./tests/load-json/samp.json"}, types.Runtime{
			Gamemodes: []string{
				"rivershell",
				"baserace",
			},
			RCONPassword: &[]string{"test"}[0],
			Port:         &[]int{8080}[0],
			Hostname:     &[]string{"Test"}[0],
			MaxPlayers:   &[]int{32}[0],
			Language:     &[]string{"English"}[0],
			Announce:     &[]bool{true}[0],
			RCON:         &[]bool{true}[0],
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Dir(tt.args.file)
			tt.wantCfg.WorkingDir = dir
			GenerateJSON(tt.wantCfg)

			gotCfg, err := ConfigFromJSON(tt.args.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigFromJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// because the ConfigFromJSON function does not know the dir
			tt.wantCfg.WorkingDir = ""

			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestConfigFromYAML(t *testing.T) {
	type args struct {
		file string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg types.Runtime
		wantErr bool
	}{
		{"yaml basic", args{"./tests/load-yaml/samp.yaml"}, types.Runtime{
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"yaml large", args{"./tests/load-yaml/samp.yaml"}, types.Runtime{
			Gamemodes: []string{
				"rivershell",
				"baserace",
			},
			RCONPassword: &[]string{"test"}[0],
			Port:         &[]int{8080}[0],
			Hostname:     &[]string{"Test"}[0],
			MaxPlayers:   &[]int{32}[0],
			Language:     &[]string{"English"}[0],
			Announce:     &[]bool{true}[0],
			RCON:         &[]bool{true}[0],
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Dir(tt.args.file)
			tt.wantCfg.WorkingDir = dir
			GenerateYAML(tt.wantCfg)

			gotCfg, err := ConfigFromYAML(tt.args.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigFromYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// because the ConfigFromJSON function does not know the dir
			tt.wantCfg.WorkingDir = ""

			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}
