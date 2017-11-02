package rook

import (
	"reflect"
	"testing"
)

func TestPackageFromDir(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantPkg Package
		wantErr bool
	}{
		{"build-json", args{"./tests/build-json"}, Package{
			Dependencies: []Dependency{
				Dependency("Southclaws/samp-stdlib:0.3.7-R2-2-1"),
				Dependency("Southclaws/SIF:1.6.2"),
				Dependency("Misiur/YSI-Includes"),
				Dependency("samp-incognito/samp-streamer-plugin:2.9.1"),
				Dependency("Zeex/amx_assembly"),
			}},
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := PackageFromDir(tt.args.dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("PackageFromDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotPkg, tt.wantPkg) {
				t.Errorf("PackageFromDir() = %v, want %v", gotPkg, tt.wantPkg)
			}
		})
	}
}
