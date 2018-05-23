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
		pkg      *types.Package
		wantDeps []versioning.DependencyMeta
		wantErr  bool
	}{
		{"basic", &types.Package{
			LocalPath: util.FullPath("./tests/deps-basic"),
			Dependencies: []versioning.DependencyString{
				"sampctl/samp-stdlib",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			}, false},
		{"circular", &types.Package{
			LocalPath: util.FullPath("./tests/deps-cirular"),
			Dependencies: []versioning.DependencyString{
				"sampctl/AAA",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "AAA"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "BBB"},
			}, false},
		{"tag", &types.Package{
			LocalPath: util.FullPath("./tests/deps-tag"),
			Dependencies: []versioning.DependencyString{
				"sampctl/samp-stdlib:0.3z-R4",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib", Tag: "0.3z-R4"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			}, false},
		{"branch", &types.Package{
			LocalPath: util.FullPath("./tests/deps-branch"),
			Dependencies: []versioning.DependencyString{
				"pawn-lang/YSI-Includes@5.x",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "YSI-Includes", Branch: "5.x"},
				versioning.DependencyMeta{Site: "github.com", User: "oscar-broman", Repo: "md-sort"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "Y-Less", Repo: "code-parse.inc"},
				versioning.DependencyMeta{Site: "github.com", User: "Y-Less", Repo: "indirection"},
				versioning.DependencyMeta{Site: "github.com", User: "Zeex", Repo: "amx_assembly"},
			}, false},
		{"commit", &types.Package{
			LocalPath: util.FullPath("./tests/deps-commit"),
			Dependencies: []versioning.DependencyString{
				"sampctl/pawn-stdlib#7a13c662e619a478b0e8d1d6d113e3aa41cb6d37",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib", Commit: "7a13c662e619a478b0e8d1d6d113e3aa41cb6d37"},
			}, false},
	}
	for _, tt := range tests {
		os.RemoveAll(tt.pkg.LocalPath)
		os.MkdirAll(tt.pkg.LocalPath, 0755) //nolint

		t.Run(tt.name, func(t *testing.T) {
			err := EnsureDependencies(context.Background(), gh, tt.pkg, nil, runtime.GOOS, "./tests/cache")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDeps, tt.pkg.AllDependencies)
		})
	}
}
