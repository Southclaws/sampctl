package rook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	git "gopkg.in/src-d/go-git.v4"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackage_EnsureDependencies(t *testing.T) {
	tests := []struct {
		name     string
		pcx      PackageContext
		wantDeps []versioning.DependencyMeta
		wantErr  bool
	}{
		{"basic", PackageContext{
			Package: types.Package{
				LocalPath:      util.FullPath("./tests/deps-basic"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
			},
			AllDependencies: []versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			},
		},
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			},
			false},
	}
	for _, tt := range tests {
		os.RemoveAll(tt.pcx.Package.LocalPath)
		os.MkdirAll(tt.pcx.Package.LocalPath, 0755) //nolint

		tt.pcx.GitHub = gh
		tt.pcx.Platform = runtime.GOOS
		tt.pcx.CacheDir = "./tests/cache"

		t.Run(tt.name, func(t *testing.T) {
			err := tt.pcx.EnsureDependencies(context.Background(), true)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDeps, tt.pcx.AllDependencies)
		})
	}
}

func TestPackageContext_EnsurePackage(t *testing.T) {
	type args struct {
		meta        versioning.DependencyMeta
		forceUpdate bool
	}
	tests := []struct {
		name          string
		args          args
		wantSha       string
		wantResources []string
		wantErr       bool
	}{
		{"commit", args{versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib", Commit: "7a13c662e619a478b0e8d1d6d113e3aa41cb6d37"}, false},
			"7a13c662e619a478b0e8d1d6d113e3aa41cb6d37", nil, false},
		{"tag", args{versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib", Tag: "0.3z-R4"}, false},
			"de2ed6d59f0304dab726588afd3b6f6df77ca87d", nil, false},
		{"branch", args{versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-errors", Branch: "v2"}, false},
			"", nil, false},
		{"resource", args{versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "package-resource-test"}, false},
			"", []string{"package-resource-test-07ad0b/include.inc"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pcxWorkspace := util.FullPath("./tests/deps-" + tt.name)
			pcxVendor := filepath.Join(pcxWorkspace, "dependencies")
			pcx := PackageContext{
				CacheDir: "./tests/cache",
				GitHub:   gh,
				GitAuth:  gitAuth,
				Platform: "linux",
				Package: types.Package{
					LocalPath:      pcxWorkspace,
					Vendor:         pcxVendor,
					DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
				},
			}

			err := pcx.EnsurePackage(tt.args.meta, tt.args.forceUpdate)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// don't check empty shas
			// some dependency modes aren't static (such as branches)
			if tt.wantSha != "" {
				path := filepath.Join(pcxWorkspace, "dependencies", tt.args.meta.Repo)
				assert.True(t, util.Exists(path))
				repo, err := git.PlainOpen(path)
				if err != nil {
					t.Error(err)
					t.FailNow()
				}
				ref, err := repo.Head()
				if err != nil {
					t.Error(err)
					t.FailNow()
				}
				assert.Equal(t, tt.wantSha, ref.Hash().String())
			}

			if len(tt.wantResources) > 0 {
				for _, resPath := range tt.wantResources {
					fmt.Println("checking:", filepath.Join(pcxVendor, ".resources", resPath))
					assert.True(t, util.Exists(filepath.Join(pcxVendor, ".resources", resPath)))
				}
			}
		})
	}
}
