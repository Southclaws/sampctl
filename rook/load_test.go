package rook

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
			local:  "./tests/build-json",
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []DependencyString{
				DependencyString("Southclaws/samp-stdlib:0.3.7-R2-2-1"),
				DependencyString("Southclaws/SIF:1.6.2"),
				DependencyString("Misiur/YSI-Includes"),
				DependencyString("samp-incognito/samp-streamer-plugin:2.9.1"),
				DependencyString("Zeex/amx_assembly"),
				DependencyString("Zeex/samp-plugin-crashdetect/include"),
			}},
			false},
		{"build-yaml", args{"./tests/build-yaml"}, Package{
			local:  "./tests/build-yaml",
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []DependencyString{
				DependencyString("Southclaws/samp-stdlib:0.3.7-R2-2-1"),
				DependencyString("Southclaws/SIF:1.6.2"),
				DependencyString("Misiur/YSI-Includes"),
				DependencyString("samp-incognito/samp-streamer-plugin:2.9.1"),
				DependencyString("Zeex/amx_assembly"),
			}},
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := PackageFromDir(tt.args.dir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantPkg, gotPkg)
		})
	}
}
