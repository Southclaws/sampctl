package pawnpackage_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"

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
	_ = godotenv.Load("../.env", "../../.env")

	token := os.Getenv("FULL_ACCESS_GITHUB_TOKEN")
	if len(token) == 0 {
		fmt.Println("No token in `FULL_ACCESS_GITHUB_TOKEN`, skipping tests.")
		return
	}
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})))

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
		pcxWorkspace := fs.MustAbs("./tests/build-auto-" + tt.name)
		pcxVendor := filepath.Join(pcxWorkspace, "dependencies")

		err := os.MkdirAll(filepath.Join(pcxWorkspace, "gamemodes"), 0o700)
		if err != nil {
			panic(err)
		}

		err = os.WriteFile(filepath.Join(pcxWorkspace, tt.args.pkg.Entry), tt.sourceCode, 0o700)
		if err != nil {
			panic(err)
		}

		pcx := pkgcontext.PackageContext{
			CacheDir:        "./tests/cache",
			GitHub:          gh,
			GitAuth:         gitAuth,
			Platform:        runtime.GOOS,
			Package:         tt.args.pkg,
			AllDependencies: tt.args.dependencies,
		}

		pcx.Package.LocalPath = pcxWorkspace
		pcx.Package.Vendor = pcxVendor
		pcx.Package.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "local"}

		t.Run(tt.name, func(t *testing.T) {
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
