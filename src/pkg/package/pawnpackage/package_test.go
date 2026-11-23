package pawnpackage_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestPackageBuildWithDependencies(t *testing.T) {
	stdlib := []versioning.DependencyMeta{
		{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
		{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
	}
	for _, tc := range []struct {
		name         string
		source       string
		builds       []*build.Config
		dependencies []versioning.DependencyMeta
	}{
		{
			name: "no dependencies", source: "main(){}",
			builds: []*build.Config{{Name: "build", Version: "3.10.10"}},
		},
		{
			name: "standard library",
			source: `#include <a_samp>
			main() {print("hi");}`,
			builds:       []*build.Config{{Name: "build", Version: "3.10.10"}},
			dependencies: stdlib,
		},
		{
			name: "standard library and UUID",
			source: `#include <a_samp>
			#include <uuid>
			main() {}`,
			dependencies: append([]versioning.DependencyMeta{
				stdlib[0], stdlib[1],
			}, versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-uuid"}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			entry := filepath.Join("gamemodes", "test.pwn")
			source := filepath.Join(root, entry)
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o700))
			require.NoError(t, os.WriteFile(source, []byte(tc.source), 0o700))

			pcx := pkgcontext.PackageContext{
				Package: pawnpackage.Package{
					LocalPath: root,
					Vendor:    filepath.Join(root, "dependencies"),
					Entry:     entry,
					Output:    filepath.Join("gamemodes", "test.amx"),
					Build:     &build.Config{Compiler: build.CompilerConfig{Path: newPackageTestCompilerDir(t)}},
					Builds:    tc.builds,
				},
				PackageServices: pkgcontext.PackageServices{
					CacheDir: seedPawnPackageFixtureCache(t),
					Platform: runtime.GOOS,
				},
				PackageResolvedState: pkgcontext.PackageResolvedState{AllDependencies: tc.dependencies},
			}
			pcx.Package.SetDependencyMeta(versioning.DependencyMeta{User: "local", Repo: "local"})

			problems, _, err := pcx.Build(t.Context(), pkgcontext.BuildOptions{Name: "build", Ensure: true})

			require.NoError(t, err)
			assert.Equal(t, build.Problems(nil), problems)
		})
	}
}

func TestPackageStringIncludesDependencyVersion(t *testing.T) {
	pkg := pawnpackage.Package{}
	pkg.SetDependencyMeta(versioning.DependencyMeta{
		Site: "github.com",
		User: "pawn-lang",
		Repo: "samp-stdlib",
		Tag:  "0.3.7-R2-2-1",
	})

	assert.Equal(t, "github.com/pawn-lang/samp-stdlib:0.3.7-R2-2-1", pkg.String())
}
