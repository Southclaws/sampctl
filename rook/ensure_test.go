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
				LocalPath: util.FullPath("./tests/deps-basic"),
			},
			AllDependencies: []versioning.DependencyMeta{
				{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			},
		},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			}, false},
		// {"circular", PackageContext{
		// 	Package: types.Package{
		// 		LocalPath: util.FullPath("./tests/deps-cirular"),
		// 		Dependencies: []versioning.DependencyString{
		// 			"sampctl/AAA",
		// 		}},
		// },
		// 	[]versioning.DependencyMeta{
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "AAA"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "BBB"},
		// 	}, false},
		// {"tag", PackageContext{
		// 	Package: types.Package{
		// 		LocalPath: util.FullPath("./tests/deps-tag"),
		// 		Dependencies: []versioning.DependencyString{
		// 			"sampctl/samp-stdlib:0.3z-R4",
		// 		}},
		// },
		// 	[]versioning.DependencyMeta{
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib", Tag: "0.3z-R4"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
		// 	}, false},
		// {"branch", PackageContext{
		// 	Package: types.Package{
		// 		LocalPath: util.FullPath("./tests/deps-branch"),
		// 		Dependencies: []versioning.DependencyString{
		// 			"pawn-lang/YSI-Includes@5.x",
		// 		}},
		// },
		// 	[]versioning.DependencyMeta{
		// 		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "YSI-Includes", Branch: "5.x"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "oscar-broman", Repo: "md-sort"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "Y-Less", Repo: "code-parse.inc"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "Y-Less", Repo: "indirection"},
		// 		versioning.DependencyMeta{Site: "github.com", User: "Zeex", Repo: "amx_assembly"},
		// 	}, false},
		// {"commit", PackageContext{
		// 	Package: types.Package{
		// 		LocalPath: util.FullPath("./tests/deps-commit"),
		// 		Dependencies: []versioning.DependencyString{
		// 			"sampctl/pawn-stdlib#7a13c662e619a478b0e8d1d6d113e3aa41cb6d37",
		// 		}},
		// },
		// 	[]versioning.DependencyMeta{
		// 		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib", Commit: "7a13c662e619a478b0e8d1d6d113e3aa41cb6d37"},
		// 	}, false},
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
