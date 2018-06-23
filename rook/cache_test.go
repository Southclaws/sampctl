package rook

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestEnsureDependenciesCached(t *testing.T) {
	type args struct {
		pkg      types.Package
		platform string
		cacheDir string
		auth     transport.AuthMethod
	}
	tests := []struct {
		name                string
		args                args
		wantAllDependencies []versioning.DependencyMeta
		wantAllIncludePaths []string
		wantAllPlugins      []versioning.DependencyMeta
		wantErr             bool
	}{
		{"basic", args{
			types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/deps-basic"),
				DependencyMeta: versioning.DependencyMeta{User: "local", Repo: "local"},
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
				},
			},
			"linux",
			"./tests/cache",
			gitAuth,
		},
			[]versioning.DependencyMeta{
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "samp-stdlib"},
				versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "pawn-stdlib"},
			},
			nil,
			nil,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.RemoveAll(tt.args.pkg.LocalPath)
			os.MkdirAll(tt.args.pkg.LocalPath, 0755) //nolint

			gotAllDependencies, gotAllIncludePaths, gotAllPlugins, err := EnsureDependenciesCached(tt.args.pkg, tt.args.platform, tt.args.cacheDir, tt.args.auth)
			if tt.wantErr {
				assert.Equal(t, tt.wantErr, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantAllDependencies, gotAllDependencies)
			assert.Equal(t, tt.wantAllIncludePaths, gotAllIncludePaths)
			assert.Equal(t, tt.wantAllPlugins, gotAllPlugins)
		})
	}
}
