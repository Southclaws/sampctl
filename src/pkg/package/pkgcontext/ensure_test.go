package pkgcontext

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestPackage_EnsureDependencies(t *testing.T) {
	tests := []struct {
		name     string
		pcx      PackageContext
		ctx      func() context.Context
		wantDeps []versioning.DependencyMeta
		wantErr  bool
		wantIs   error
	}{
		{
			"basic",
			PackageContext{
				Package: pawnpackage.Package{
					User: "local", Repo: "local",
				},
				PackageResolvedState: PackageResolvedState{AllDependencies: []versioning.DependencyMeta{
					{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
					{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
				}},
			},
			func() context.Context { return context.Background() },
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
				{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
			},
			false,
			nil,
		},
		{
			"canceled context",
			PackageContext{
				Package: pawnpackage.Package{
					User: "local", Repo: "local",
				},
				PackageResolvedState: PackageResolvedState{AllDependencies: []versioning.DependencyMeta{
					{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
				}},
			},
			func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
			},
			true,
			context.Canceled,
		},
	}

	for _, tt := range tests {
		tt.pcx.GitHub = gh
		tt.pcx.Platform = runtime.GOOS
		tt.pcx.CacheDir = fixtureCacheDir

		t.Run(tt.name, func(t *testing.T) {
			tt.pcx.Package.LocalPath = t.TempDir()

			err := tt.pcx.EnsureDependencies(tt.ctx(), true)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs)
			}

			assert.Equal(t, tt.wantDeps, tt.pcx.AllDependencies)
		})
	}
}

func TestEnsurePackageInstallsSelectedRevisionOrResources(t *testing.T) {
	for _, tc := range []struct {
		name          string
		meta          versioning.DependencyMeta
		wantCommit    string
		wantResources []string
	}{
		{
			name: "commit", wantCommit: fixturePawnStdlibCommit,
			meta: versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib", Commit: fixturePawnStdlibCommit},
		},
		{
			name: "tag", wantCommit: fixtureSampStdlibCommit,
			meta: versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib", Tag: "0.3z-R4"},
		},
		{
			name: "branch",
			meta: versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-errors", Branch: "v2"},
		},
		{
			name: "resource", wantResources: []string{"package-resource-test-07ad0b/include.inc"},
			meta: versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "package-resource-test"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			vendor := filepath.Join(root, "dependencies")
			pcx := PackageContext{
				Package: pawnpackage.Package{LocalPath: root, Vendor: vendor, User: "local", Repo: "local"},
				PackageServices: PackageServices{
					CacheDir: fixtureCacheDir,
					GitHub:   gh,
					GitAuth:  gitAuth,
					Platform: "linux",
				},
			}

			err := pcx.EnsurePackage(tc.meta, false)

			require.NoError(t, err)
			if tc.wantCommit != "" {
				path := filepath.Join(vendor, tc.meta.Repo)
				assert.DirExists(t, path)
				repo, err := git.PlainOpen(path)
				require.NoError(t, err)
				head, err := repo.Head()
				require.NoError(t, err)
				assert.Equal(t, tc.wantCommit, head.Hash().String())
			}
			for _, path := range tc.wantResources {
				assert.FileExists(t, filepath.Join(vendor, ".resources", path))
			}
		})
	}
}
