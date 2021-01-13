package pkgcontext

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/pawnpackage"
	"github.com/Southclaws/sampctl/run"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackageFromDir(t *testing.T) {
	type args struct {
		dir string
	}
	tests := []struct {
		name    string
		args    args
		wantPkg pawnpackage.Package
		wantErr bool
	}{
		{"load-json", args{"tests/load-json"}, pawnpackage.Package{
			Parent:         true,
			LocalPath:      "tests/load-json",
			Vendor:         "tests/load-json/dependencies",
			Format:         "json",
			DependencyMeta: versioning.DependencyMeta{User: "<none>", Repo: "<local>"},
			Entry:          "gamemodes/test.pwn",
			Output:         "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"pawn-lang/samp-stdlib:0.3.7-R2-2-1",
				"Southclaws/pawn-errors:1.2.3",
			},
			Runtime: &run.Runtime{
				Echo:         nil,
				Version:      "0.3.7",
				Platform:     runtime.GOOS,
				RCONPassword: &[]string{"password"}[0],
				Port:         &[]int{7777}[0],
				Mode:         run.Server,
			}},
			false},
		{"load-yaml", args{"tests/load-yaml"}, pawnpackage.Package{
			Parent:         true,
			LocalPath:      "tests/load-yaml",
			Vendor:         "tests/load-yaml/dependencies",
			Format:         "yaml",
			DependencyMeta: versioning.DependencyMeta{User: "<none>", Repo: "<local>"},
			Entry:          "gamemodes/test.pwn",
			Output:         "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"pawn-lang/samp-stdlib:0.3.7-R2-2-1",
				"Southclaws/pawn-errors:1.2.3",
			},
			Runtime: &run.Runtime{
				Echo:         nil,
				Version:      "0.3.7",
				Platform:     runtime.GOOS,
				RCONPassword: &[]string{"password"}[0],
				Port:         &[]int{7777}[0],
				Mode:         run.Server,
			}},
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPcx, err := NewPackageContext(gh, gitAuth, true, tt.args.dir, runtime.GOOS, "./tests/cache", "")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.wantPkg.Vendor = filepath.FromSlash(tt.wantPkg.Vendor)

			assert.Equal(t, tt.wantPkg, gotPcx.Package)
		})
	}
}
