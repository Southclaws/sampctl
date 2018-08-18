package rook

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackage_Build(t *testing.T) {
	type args struct {
		pkg          types.Package
		build        string
		ensure       bool
		dependencies []versioning.DependencyMeta
	}
	tests := []struct {
		name         string
		sourceCode   []byte
		args         args
		wantProblems types.BuildProblems
		wantErr      bool
	}{
		{
			"bare", []byte(`main(){}`), args{
				types.Package{
					Parent:         true,
					LocalPath:      util.FullPath("./tests/build-auto-bare"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "bare"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
					Builds: []*types.BuildConfig{
						{Name: "build", Version: "3.10.4"},
					},
				},
				"build", true, []versioning.DependencyMeta{},
			}, nil, false,
		},
		{
			"stdlib", []byte(`#include <a_samp>
			main() {print("hi");}`,
			), args{
				types.Package{
					Parent:         true,
					LocalPath:      util.FullPath("./tests/build-auto-stdlib"),
					DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "stdlib"},
					Entry:          "gamemodes/test.pwn",
					Output:         "gamemodes/test.amx",
					Builds: []*types.BuildConfig{
						{Name: "build", Version: "3.10.4"},
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
				types.Package{
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

		pcx := PackageContext{
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
