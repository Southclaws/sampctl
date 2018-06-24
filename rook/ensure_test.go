package rook

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

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
			}, false},
		{"tag", PackageContext{
			Package: types.Package{
				LocalPath:      util.FullPath("./tests/deps-tag"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
			},
			AllDependencies: []versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "samp-stdlib", Tag: "0.3z-R4"},
			},
		},
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "samp-stdlib", Tag: "0.3z-R4"},
			}, false},
		{"branch", PackageContext{
			Package: types.Package{
				LocalPath:      util.FullPath("./tests/deps-branch"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
			},
			AllDependencies: []versioning.DependencyMeta{
				{Site: "github.com", User: "pawn-lang", Repo: "YSI-Includes", Branch: "5.x"},
			},
		},
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "pawn-lang", Repo: "YSI-Includes", Branch: "5.x"},
			}, false},
		{"commit", PackageContext{
			Package: types.Package{
				LocalPath:      util.FullPath("./tests/deps-commit"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
			},
			AllDependencies: []versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib", Commit: "7a13c662e619a478b0e8d1d6d113e3aa41cb6d37"},
			},
		},
			[]versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib", Commit: "7a13c662e619a478b0e8d1d6d113e3aa41cb6d37"},
			}, false},
	}
	for _, tt := range tests {
		os.RemoveAll(tt.pcx.Package.LocalPath)
		os.MkdirAll(tt.pcx.Package.LocalPath, 0755) //nolint

		tt.pcx.GitHub = gh
		tt.pcx.Platform = runtime.GOOS
		tt.pcx.CacheDir = "./tests/cache"

		t.Run(tt.name, func(t *testing.T) {
			err := tt.pcx.EnsureDependencies(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDeps, tt.pcx.AllDependencies)
		})
	}
}
