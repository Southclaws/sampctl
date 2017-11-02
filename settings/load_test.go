package settings

import (
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
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
