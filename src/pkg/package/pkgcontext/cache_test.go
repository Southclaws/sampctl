package pkgcontext

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestEnsureDependenciesCached(t *testing.T) {
	sampStdlib := versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"}
	pawnStdlib := versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"}
	pawnRequests := versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-requests"}

	for _, tc := range []struct {
		name         string
		dependencies []versioning.DependencyString
		want         []versioning.DependencyMeta
	}{
		{
			name:         "includes transitive standard library",
			dependencies: []versioning.DependencyString{"pawn-lang/samp-stdlib"},
			want:         []versioning.DependencyMeta{sampStdlib, pawnStdlib},
		},
		{
			name:         "includes plugin alongside standard libraries",
			dependencies: []versioning.DependencyString{"pawn-lang/samp-stdlib", "Southclaws/pawn-requests"},
			want:         []versioning.DependencyMeta{sampStdlib, pawnStdlib, pawnRequests},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pcx := PackageContext{
				Package: pawnpackage.Package{
					Parent:       true,
					LocalPath:    t.TempDir(),
					User:         "local",
					Repo:         "local",
					Dependencies: tc.dependencies,
				},
				PackageServices: PackageServices{
					Platform: "linux",
					CacheDir: fixtureCacheDir,
					GitHub:   gh,
					GitAuth:  gitAuth,
				},
			}

			err := pcx.EnsureDependenciesCached()

			require.NoError(t, err)
			require.Equal(t, tc.want, pcx.AllDependencies)
		})
	}
}
