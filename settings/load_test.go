package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigFromDirectory(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantCfg Config
		wantErr bool
	}{
		{"both basic", args{"./tests/load-both"}, Config{
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"both large", args{"./tests/load-yaml"}, Config{
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
			tt.wantCfg.GenerateJSON(tt.args.dir)
			tt.wantCfg.GenerateYAML(tt.args.dir)

			gotCfg, err := ConfigFromDirectory(tt.args.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigFromDirectory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotCfg, tt.wantCfg) {
				t.Errorf("ConfigFromDirectory() = %v, want %v", gotCfg, tt.wantCfg)
			}
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
		wantCfg Config
		wantErr bool
	}{
		{"json basic", args{"./tests/load-json/samp.json"}, Config{
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"json large", args{"./tests/load-json/samp.json"}, Config{
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
			tt.wantCfg.GenerateJSON(filepath.Dir(tt.args.file))

			gotCfg, err := ConfigFromJSON(tt.args.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigFromJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotCfg, tt.wantCfg) {
				t.Errorf("ConfigFromJSON() = %v, want %v", gotCfg, tt.wantCfg)
			}
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
		wantCfg Config
		wantErr bool
	}{
		{"yaml basic", args{"./tests/load-yaml/samp.yaml"}, Config{
			Gamemodes:    []string{"rivershell"},
			RCONPassword: &[]string{"hello"}[0],
		}, false},
		{"yaml large", args{"./tests/load-yaml/samp.yaml"}, Config{
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
			tt.wantCfg.GenerateYAML(filepath.Dir(tt.args.file))

			gotCfg, err := ConfigFromYAML(tt.args.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfigFromYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotCfg, tt.wantCfg) {
				t.Errorf("ConfigFromYAML() = %v, want %v", gotCfg, tt.wantCfg)
			}
		})
	}
}
