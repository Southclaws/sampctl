package run

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuntimeFromDir(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg Runtime
		wantErr bool
	}{
		{"both basic", args{"./tests/load-both"}, Runtime{
			Format:       "json",
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"both large", args{"./tests/load-yaml"}, Runtime{
			Format: "json",
			Gamemodes: []string{
				"rivershell",
				"baserace",
			},
			Plugins: []Plugin{
				"streamer",
				"zeex/samp-plugin-crashdetect",
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

			_ = tt.wantCfg.ToJSON()
			_ = tt.wantCfg.ToYAML()

			gotCfg, err := RuntimeFromDir(tt.args.dir)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestRuntimeFromJSON(t *testing.T) {
	type args struct {
		file string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg Runtime
		wantErr bool
	}{
		{"json basic", args{"./tests/load-json/samp.json"}, Runtime{
			Format:       "json",
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"json large", args{"./tests/load-json/samp.json"}, Runtime{
			Format: "json",
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
			_ = tt.wantCfg.ToJSON()

			gotCfg, err := RuntimeFromJSON(tt.args.file)
			if (err != nil) != tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// because the RuntimeFromJSON function does not know the dir
			tt.wantCfg.WorkingDir = ""

			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestRuntimeFromYAML(t *testing.T) {
	type args struct {
		file string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg Runtime
		wantErr bool
	}{
		{"yaml basic", args{"./tests/load-yaml/samp.yaml"}, Runtime{
			Format:       "yaml",
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"yaml large", args{"./tests/load-yaml/samp.yaml"}, Runtime{
			Format: "yaml",
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
			_ = tt.wantCfg.ToYAML()

			gotCfg, err := RuntimeFromYAML(tt.args.file)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// because the RuntimeFromYAML function does not know the dir
			tt.wantCfg.WorkingDir = ""

			assert.Equal(t, tt.wantCfg, gotCfg)
		})
	}
}

func TestRuntimeToJSON(t *testing.T) {
	tests := []struct {
		name    string
		config  Runtime
		want    []byte
		wantErr bool
	}{
		{
			"minimal",
			Runtime{
				WorkingDir: "./tests/generate-json",
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
			},
			[]byte(`{
	"gamemodes": [
		"rivershell",
		"baserace"
	],
	"rcon_password": "test",
	"port": 8080,
	"hostname": "Test",
	"maxplayers": 32,
	"language": "English",
	"announce": true,
	"rcon": true
}`),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ToJSON()
			assert.NoError(t, err)

			contents, err := ioutil.ReadFile(filepath.Join(tt.config.WorkingDir, "samp.json"))
			assert.NoError(t, err)

			assert.Equal(t, string(tt.want), string(contents))
		})
	}
}

func TestRuntimeToYAML(t *testing.T) {
	tests := []struct {
		name    string
		config  Runtime
		want    []byte
		wantErr bool
	}{
		{
			"minimal",
			Runtime{
				WorkingDir: "./tests/generate-yaml",
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
			},
			[]byte(`gamemodes:
    - rivershell
    - baserace
rcon_password: test
port: 8080
hostname: Test
maxplayers: 32
language: English
announce: true
rcon: true
`),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ToYAML()
			assert.NoError(t, err)

			contents, err := ioutil.ReadFile(filepath.Join(tt.config.WorkingDir, "samp.yaml"))
			assert.NoError(t, err)

			assert.Equal(t, string(tt.want), string(contents))
		})
	}
}
