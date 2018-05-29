package rook

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackage_Build(t *testing.T) {
	type args struct {
		pkg    *types.Package
		build  string
		ensure bool
	}
	tests := []struct {
		name         string
		sourceCode   []byte
		args         args
		wantProblems types.BuildProblems
		wantErr      bool
	}{
		{
			"bare", []byte(`main(){}`), args{&types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/build-auto-bare"),
				DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "bare"},
				Entry:          "gamemodes/test.pwn",
				Output:         "gamemodes/test.amx",
				Dependencies:   []versioning.DependencyString{},
				Builds: []*types.BuildConfig{
					{Name: "build", Version: "3.10.4"},
				},
			}, "build", true}, nil, false,
		},
		{
			"stdlib", []byte(`#include <a_samp>
			main() {print("hi");}`,
			), args{&types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/build-auto-stdlib"),
				DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "stdlib"},
				Entry:          "gamemodes/test.pwn",
				Output:         "gamemodes/test.amx",
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
				},
				Builds: []*types.BuildConfig{
					{Name: "build", Version: "3.10.4"},
				},
			}, "build", true}, nil, false,
		},
		// {
		// 	"deep", []byte(`#include <a_samp>
		// 	#include <actions>
		// 	main() { print("actions"); }`,
		// 	), args{&types.Package{
		// 		Parent:         true,
		// 		LocalPath:      util.FullPath("./tests/build-auto-deep"),
		// 		DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "deep"},
		// 		Entry:          "gamemodes/test.pwn",
		// 		Output:         "gamemodes/test.amx",
		// 		Dependencies: []versioning.DependencyString{
		// 			"sampctl/samp-stdlib",
		// 			"ScavengeSurvive/actions",
		// 		},
		// 	}, "build", true}, nil, false,
		// },
		// {
		// 	"dev", []byte(`#include <a_samp>
		// 		#include <actions>
		// 		#include <test-boilerplate>
		// 		main() { print("actions"); }`,
		// 	), args{&types.Package{
		// 		Parent:         true,
		// 		LocalPath:      util.FullPath("./tests/build-auto-deep"),
		// 		DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "deep"},
		// 		Entry:          "gamemodes/test.pwn",
		// 		Output:         "gamemodes/test.amx",
		// 		Dependencies: []versioning.DependencyString{
		// 			"sampctl/samp-stdlib",
		// 			"ScavengeSurvive/actions",
		// 		},
		// 		Development: []versioning.DependencyString{
		// 			"ScavengeSurvive/test-boilerplate",
		// 		},
		// 	}, "build", true}, nil, false,
		// },
		{
			"custominc", []byte(`#include <a_samp>
			#include <YSI\y_utils>
			main() {}`,
			), args{&types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/build-auto-custominc"),
				DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "custominc"},
				Entry:          "gamemodes/test.pwn",
				Output:         "gamemodes/test.amx",
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
				},
				Builds: []*types.BuildConfig{
					{
						Name:    "build",
						Version: "3.10.4",
						Includes: []string{
							"../build-auto-deep/dependencies/amx_assembly",
							"../build-auto-deep/dependencies/YSI-Includes",
						},
						Args: []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
					},
				},
			}, "build", true}, nil, false,
		},
		{
			"resourceinc", []byte(`#include <a_samp>
			#include <a_mysql>
			main() {}`,
			), args{&types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/build-auto-resourceinc"),
				DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "resourceinc"},
				Entry:          "gamemodes/test.pwn",
				Output:         "gamemodes/test.amx",
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
					"pBlueG/SA-MP-MySQL",
				},
			}, "build", true}, nil, false,
		},
		{
			"colandreasinc", []byte(`#include <a_samp>
			#include <colandreas>
			main() {}`,
			), args{&types.Package{
				Parent:         true,
				LocalPath:      util.FullPath("./tests/build-auto-colandreasinc"),
				DependencyMeta: versioning.DependencyMeta{User: "test", Repo: "colandreasinc"},
				Entry:          "gamemodes/test.pwn",
				Output:         "gamemodes/test.amx",
				Dependencies: []versioning.DependencyString{
					"sampctl/samp-stdlib",
					"Pottus/ColAndreas",
				},
			}, "build", true}, nil, false,
		},
	}
	for _, tt := range tests {
		err := os.MkdirAll(filepath.Join(tt.args.pkg.LocalPath, "gamemodes"), 0755)
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(filepath.Join(tt.args.pkg.LocalPath, tt.args.pkg.Entry), tt.sourceCode, 0755)
		if err != nil {
			panic(err)
		}

		t.Run(tt.name, func(t *testing.T) {
			gotProblems, _, err := Build(context.Background(), gh, nil, tt.args.pkg, tt.args.build, "tests/cache", "linux", tt.args.ensure, false, false, "")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.wantProblems, gotProblems)
		})
	}
}
