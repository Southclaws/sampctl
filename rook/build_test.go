package rook

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
	"github.com/stretchr/testify/assert"
)

func TestPackage_Build(t *testing.T) {
	type args struct {
		pkg    *types.Package
		build  string
		ensure bool
	}
	tests := []struct {
		name       string
		sourceCode []byte
		args       args
		wantOutput string
		wantErr    bool
	}{
		{"stdlib", []byte(`#include <a_samp>
			main() {print("hi");}`,
		), args{&types.Package{
			Parent: true,
			Local:  util.FullPath("./tests/build-auto-stdlib"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
			},
			Builds: []types.BuildConfig{
				{Name: "build", Version: "3.10.4"},
			},
		}, "build", true}, "gamemodes/test.amx", false},
		{"deep", []byte(`#include <a_samp>
			#include <actions>
			main() { print("actions"); }`,
		), args{&types.Package{
			Parent: true,
			Local:  util.FullPath("./tests/build-auto-deep"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
				"ScavengeSurvive/actions",
			},
		}, "build", true}, "gamemodes/test.amx", false},
		{"custominc", []byte(`#include <a_samp>
			#include <YSI\y_utils>
			main() {}`,
		), args{&types.Package{
			Parent: true,
			Local:  util.FullPath("./tests/build-auto-custominc"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
			},
			Builds: []types.BuildConfig{
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
		}, "build", true}, "gamemodes/test.amx", false},
	}
	for _, tt := range tests {
		err := os.MkdirAll(filepath.Join(tt.args.pkg.Local, "gamemodes"), 0755)
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(filepath.Join(tt.args.pkg.Local, tt.args.pkg.Entry), tt.sourceCode, 0755)
		if err != nil {
			panic(err)
		}

		t.Run(tt.name, func(t *testing.T) {
			gotOutput, err := Build(tt.args.pkg, tt.args.build, tt.args.ensure)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.wantOutput, gotOutput)
		})
	}
}
