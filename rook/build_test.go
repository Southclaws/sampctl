package rook

import (
	"testing"

	"github.com/Southclaws/sampctl/util"
)

func TestPackage_Build(t *testing.T) {
	type args struct {
		version string
	}
	tests := []struct {
		name       string
		pkg        Package
		args       args
		wantOutput string
		wantErr    bool
	}{
		{"basic", Package{
			local:  util.FullPath("./tests/build"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []Dependency{
				Dependency("Southclaws/samp-stdlib:0.3.7-R2-2-1"),
				Dependency("Southclaws/SIF:1.6.2"),
				Dependency("Misiur/YSI-Includes"),
				Dependency("samp-incognito/samp-streamer-plugin:2.9.1"),
				Dependency("Zeex/amx_assembly"),
			},
		}, args{"3.10.3"}, "gamemodes/test.amx", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutput, err := tt.pkg.Build(tt.args.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("Package.Build() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOutput != tt.wantOutput {
				t.Errorf("Package.Build() = %v, want %v", gotOutput, tt.wantOutput)
			}
		})
	}
}
