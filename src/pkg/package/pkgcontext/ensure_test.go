package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestPackage_EnsureDependencies(t *testing.T) {
	tests := []struct {
		name     string
		pcx      PackageContext
		wantDeps []versioning.DependencyMeta
		wantErr  bool
	}{
		{
			"basic",
			PackageContext{
				Package: pawnpackage.Package{
					LocalPath: fs.MustAbs("./tests/deps-basic"),
					User:      "local", Repo: "local",
				},
				AllDependencies: []versioning.DependencyMeta{
					{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
					{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
				},
			},
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
				{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
			},
			false,
		},
	}
	for _, tt := range tests {
		os.RemoveAll(tt.pcx.Package.LocalPath)
		os.MkdirAll(tt.pcx.Package.LocalPath, 0o700) //nolint

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
	commitMeta := versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib", Commit: fixturePawnStdlibCommit}
	tagMeta := versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib", Tag: "0.3z-R4"}
	branchMeta := versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-errors", Branch: "v2"}
	resourceMeta := versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "package-resource-test"}

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
		{
			"commit",
			args{commitMeta, false},
			fixturePawnStdlibCommit, nil, false,
		},
		{
			"tag",
			args{tagMeta, false},
			fixtureSampStdlibCommit, nil, false,
		},
		{
			"branch",
			args{branchMeta, false},
			"", nil, false,
		},
		{
			"resource",
			args{resourceMeta, false},
			"",
			[]string{"package-resource-test-07ad0b/include.inc"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pcxWorkspace := t.TempDir()
			pcxVendor := filepath.Join(pcxWorkspace, "dependencies")
			pcx := PackageContext{
				CacheDir: "./tests/cache",
				GitHub:   gh,
				GitAuth:  gitAuth,
				Platform: "linux",
				Package: pawnpackage.Package{
					LocalPath: pcxWorkspace,
					Vendor:    pcxVendor,
					User:      "local", Repo: "local",
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
				assert.True(t, fs.Exists(path))
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
					assert.True(t, fs.Exists(filepath.Join(pcxVendor, ".resources", resPath)))
				}
			}
		})
	}
}
