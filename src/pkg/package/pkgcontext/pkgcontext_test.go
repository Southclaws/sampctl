package pkgcontext

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestNewPackageContextLoadsProjectDefinition(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			dir := "tests/load-" + format
			want := pawnpackage.Package{
				Parent:    true,
				LocalPath: dir,
				Vendor:    filepath.Join(dir, "dependencies"),
				Format:    format,
				User:      "<none>",
				Repo:      "<local>",
				Entry:     "gamemodes/test.pwn",
				Output:    "gamemodes/test.amx",
				Dependencies: []versioning.DependencyString{
					"pawn-lang/samp-stdlib:0.3.7-R2-2-1",
					"Southclaws/pawn-errors:1.2.3",
				},
			}

			got, err := NewPackageContext(NewPackageContextOptions{
				GitHub:   gh,
				Auth:     gitAuth,
				Parent:   true,
				Dir:      dir,
				Platform: runtime.GOOS,
				CacheDir: fixtureCacheDir,
			})

			require.NoError(t, err)
			require.Equal(t, want, got.Package)
		})
	}
}
