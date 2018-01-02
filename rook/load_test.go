package rook

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackageFromDir(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantPkg types.Package
		wantErr bool
	}{
		{"load-json", args{"tests/load-json"}, types.Package{
			Parent:         true,
			Local:          "tests/load-json",
			Vendor:         "tests/load-json/dependencies",
			Format:         "json",
			DependencyMeta: versioning.DependencyMeta{User: "<none>", Repo: "<local>"},
			Entry:          "gamemodes/test.pwn",
			Output:         "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
				"Southclaws/SIF:1.6.2",
				"Misiur/YSI-Includes",
				"samp-incognito/samp-streamer-plugin:2.9.1",
				"Zeex/amx_assembly",
				"Zeex/samp-plugin-crashdetect/include",
			}},
			false},
		{"load-yaml", args{"tests/load-yaml"}, types.Package{
			Parent:         true,
			Local:          "tests/load-yaml",
			Vendor:         "tests/load-yaml/dependencies",
			Format:         "yaml",
			DependencyMeta: versioning.DependencyMeta{User: "<none>", Repo: "<local>"},
			Entry:          "gamemodes/test.pwn",
			Output:         "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
				"Southclaws/SIF:1.6.2",
				"Misiur/YSI-Includes",
				"samp-incognito/samp-streamer-plugin:2.9.1",
				"Zeex/amx_assembly",
			}},
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := PackageFromDir(true, tt.args.dir, "")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.wantPkg.Vendor = filepath.FromSlash(tt.wantPkg.Vendor)

			assert.Equal(t, tt.wantPkg, gotPkg)
		})
	}
}
