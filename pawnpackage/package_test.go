package pawnpackage_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/build"
	"github.com/Southclaws/sampctl/pawnpackage"
	"github.com/Southclaws/sampctl/pkgcontext"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

var gh *github.Client
var gitAuth transport.AuthMethod

func TestMain(m *testing.M) {
	godotenv.Load("../.env", "../../.env")

	token := os.Getenv("FULL_ACCESS_GITHUB_TOKEN")
	if len(token) == 0 {
		fmt.Println("No token in `FULL_ACCESS_GITHUB_TOKEN`, skipping tests.")
		return
	}
	gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})))

	err := os.MkdirAll("./tests/cache", 0700)
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
					LocalPath:      util.FullPath("./tests/build-auto-bare"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "bare"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
					Builds: []*build.Config{
						{Name: "build", Version: "3.10.10"},
					},
				},
				"build", true, []versioning.DependencyMeta{},
			}, nil, false,
		},
		{
			"stdlib", []byte(`#include <a_samp>
			main() {print("hi");}`,
			), args{
				pawnpackage.Package{
					Parent:         true,
					LocalPath:      util.FullPath("./tests/build-auto-stdlib"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "stdlib"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
					Builds: []*build.Config{
						{Name: "build", Version: "3.10.10"},
					},
				},
				"build", true,
				[]versioning.DependencyMeta{
					{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
					{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
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
					LocalPath:      util.FullPath("./tests/build-auto-requests"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "requests"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
				},
				"build", true,
				[]versioning.DependencyMeta{
					{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
					{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
					{Site: "github.com", User: "Southclaws", Repo: "pawn-uuid"},
				},
			}, nil, false,
		},
	}
	for _, tt := range tests {
		pcxWorkspace := util.FullPath("./tests/build-auto-" + tt.name)
		pcxVendor := filepath.Join(pcxWorkspace, "dependencies")

		err := os.MkdirAll(filepath.Join(pcxWorkspace, "gamemodes"), 0755)
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(filepath.Join(pcxWorkspace, tt.args.pkg.Entry), tt.sourceCode, 0755)
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
