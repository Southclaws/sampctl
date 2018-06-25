package rook

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestEnsureDependenciesCached(t *testing.T) {
	tests := []struct {
		name                string
		pcx                 PackageContext
		wantAllDependencies []versioning.DependencyMeta
		wantErr             bool
	}{
		{"basic", PackageContext{
			Package: types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/deps-basic"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
				},
			},
			Platform: "linux",
			CacheDir: "./tests/cache",
			GitAuth:  gitAuth,
		},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			},
			false,
		},
		{"plugin", PackageContext{
			Package: types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/deps-plugin"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
					"Southclaws/pawn-requests",
				},
			},
			Platform: "linux",
			CacheDir: "./tests/cache",
			GitAuth:  gitAuth,
		},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-requests"},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.RemoveAll(tt.pcx.Package.LocalPath)
			os.MkdirAll(tt.pcx.Package.LocalPath, 0755) //nolint

			tt.pcx.GitHub = gh
			tt.pcx.GitAuth = gitAuth

			err := tt.pcx.EnsureDependenciesCached()
			if tt.wantErr {
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}

			gotAllDependencies := tt.pcx.AllDependencies

			assert.Equal(t, tt.wantAllDependencies, gotAllDependencies)
		})
	}
}
