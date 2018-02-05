package rook

import (
	"os"
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
			Local: util.FullPath("./tests/deps-basic"),
			Dependencies: []versioning.DependencyString{
				"sampctl/samp-stdlib",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "https://github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "https://github.com", User: "sampctl", Repo: "pawn-stdlib"},
			}, false},
		{"circular", &types.Package{
			Local: util.FullPath("./tests/deps-cirular"),
			Dependencies: []versioning.DependencyString{
				"sampctl/AAA",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "https://github.com", User: "sampctl", Repo: "AAA"},
				versioning.DependencyMeta{Site: "https://github.com", User: "sampctl", Repo: "BBB"},
			}, false},
		{"tag", &types.Package{
			Local: util.FullPath("./tests/deps-tag"),
			Dependencies: []versioning.DependencyString{
				"sampctl/samp-stdlib:0.3z-R4",
			}},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "https://github.com", User: "sampctl", Repo: "samp-stdlib", Tag: "0.3z-R4"},
				versioning.DependencyMeta{Site: "https://github.com", User: "sampctl", Repo: "pawn-stdlib"},
			}, false},
	}
	for _, tt := range tests {
		os.MkdirAll(tt.pkg.Local, 0755) //nolint

		t.Run(tt.name, func(t *testing.T) {
			err := EnsureDependencies(tt.pkg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantDeps, tt.pkg.AllDependencies)
		})
	}
}
