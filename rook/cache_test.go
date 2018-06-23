package rook

import (
	"os"
	"reflect"
	"testing"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
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
				Parent:    true,
				LocalPath: util.FullPath("./tests/deps-basic"),
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
			[]string{},
			[]versioning.DependencyMeta{},
			false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.RemoveAll(tt.args.pkg.LocalPath)
			os.MkdirAll(tt.args.pkg.LocalPath, 0755) //nolint

			gotAllDependencies, gotAllIncludePaths, gotAllPlugins, err := EnsureDependenciesCached(tt.args.pkg, tt.args.platform, tt.args.cacheDir, tt.args.auth)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureDependenciesCached() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotAllDependencies, tt.wantAllDependencies) {
				t.Errorf("EnsureDependenciesCached() gotAllDependencies = %v, want %v", gotAllDependencies, tt.wantAllDependencies)
			}
			if !reflect.DeepEqual(gotAllIncludePaths, tt.wantAllIncludePaths) {
				t.Errorf("EnsureDependenciesCached() gotAllIncludePaths = %v, want %v", gotAllIncludePaths, tt.wantAllIncludePaths)
			}
			if !reflect.DeepEqual(gotAllPlugins, tt.wantAllPlugins) {
				t.Errorf("EnsureDependenciesCached() gotAllPlugins = %v, want %v", gotAllPlugins, tt.wantAllPlugins)
			}
		})
	}
}
