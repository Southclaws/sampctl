package pawnpackage_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

var (
	gh      *github.Client
	gitAuth transport.AuthMethod
)

func TestMain(m *testing.M) {
	err := os.MkdirAll("./tests/cache", 0o700)
	if err != nil {
		panic(err)
	}

	print.SetVerbose()

	os.Exit(m.Run())
}

func TestPackage_Build(t *testing.T) {
	type args struct {
		pkg          pawnpackage.Package
		build        string
		ensure       bool
		dependencies []versioning.DependencyMeta
	}
	tests := []struct {
		name         string
		sourceCode   []byte
		args         args
		wantProblems build.Problems
		wantErr      bool
	}{
		{
			"bare", []byte(`main(){}`), args{
				pawnpackage.Package{
					Parent:         true,
					LocalPath:      fs.MustAbs("./tests/build-auto-bare"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "bare"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
					Builds: []*build.Config{
						{Name: "build", Version: "3.10.10"},
					},
				},
				"build", true,
				[]versioning.DependencyMeta{},
			}, nil, false,
		},
		{
			"stdlib", []byte(`#include <a_samp>
			main() {print("hi");}`,
			), args{
				pawnpackage.Package{
					Parent:         true,
					LocalPath:      fs.MustAbs("./tests/build-auto-stdlib"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "stdlib"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
					Builds: []*build.Config{
						{Name: "build", Version: "3.10.10"},
					},
				},
				"build", true,
				[]versioning.DependencyMeta{
					{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
					{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
				},
			}, nil, false,
		},
		{
			"uuid", []byte(`#include <a_samp>
			#include <uuid>
			main() {}`,
			), args{
				pawnpackage.Package{
					Parent:         true,
					LocalPath:      fs.MustAbs("./tests/build-auto-requests"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "requests"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
				},
				"build", true,
				[]versioning.DependencyMeta{
					{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
					{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
					{Site: "github.com", User: "Southclaws", Repo: "pawn-uuid"},
				},
			}, nil, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := seedPawnPackageFixtureCache(t)
			pcxWorkspace := t.TempDir()
			pcxVendor := filepath.Join(pcxWorkspace, "dependencies")
			compilerDir := newPackageTestCompilerDir(t)

			err := os.MkdirAll(filepath.Join(pcxWorkspace, "gamemodes"), 0o700)
			if err != nil {
				t.Fatalf("create gamemodes dir: %v", err)
			}

			err = os.WriteFile(filepath.Join(pcxWorkspace, tt.args.pkg.Entry), tt.sourceCode, 0o700)
			if err != nil {
				t.Fatalf("write source file: %v", err)
			}

			pcx := pkgcontext.PackageContext{
				CacheDir:        cacheDir,
				GitHub:          gh,
				GitAuth:         gitAuth,
				Platform:        runtime.GOOS,
				Package:         tt.args.pkg,
				AllDependencies: tt.args.dependencies,
			}

			pcx.Package.Parent = false
			pcx.Package.LocalPath = pcxWorkspace
			pcx.Package.Vendor = pcxVendor
			pcx.Package.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "local"}
			pcx.Package.Build = &build.Config{Compiler: build.CompilerConfig{Path: compilerDir}}

			gotProblems, _, err := pcx.Build(context.Background(), tt.args.build, tt.args.ensure, false, false, "")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.wantProblems, gotProblems)
		})
	}
}
