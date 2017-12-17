package rook

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func TestPackage_Build(t *testing.T) {
	type args struct {
		build  string
		ensure bool
	}
	tests := []struct {
		name       string
		sourceCode []byte
		pkg        Package
		args       args
		wantOutput string
		wantErr    bool
	}{
		// {"stdlib", []byte(`#include <a_samp>
		// 	main() {print("hi");}
		// 	`), Package{
		// 	local:  util.FullPath("./tests/build-auto-stdlib"),
		// 	Entry:  "gamemodes/test.pwn",
		// 	Output: "gamemodes/test.amx",
		// 	Dependencies: []versioning.DependencyString{
		// 		"Southclaws/samp-stdlib:0.3.7-R2-2-1",
		// 	},
		// 	Builds: []compiler.Config{
		// 		{Name: "build", Version: "3.10.4"},
		// 	},
		// }, args{"build", true}, "gamemodes/test.amx", false},
		{"deep", []byte(`#include <a_samp>
			#include <actions>
			main() { print("actions"); }
			`), Package{
			parent: true,
			local:  util.FullPath("./tests/build-auto-deep"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []versioning.DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
				"ScavengeSurvive/actions",
			},
			Builds: []compiler.Config{
				{Name: "build", Version: "3.10.4"},
			},
		}, args{"build", true}, "gamemodes/test.amx", false},
		// {"custominc", []byte(`#include <a_samp>
		// 	main() {}
		// 	`), Package{
		// 	local:        util.FullPath("./tests/build-auto-custominc"),
		// 	Entry:        "gamemodes/test.pwn",
		// 	Output:       "gamemodes/test.amx",
		// 	Dependencies: []versioning.DependencyString{},
		// 	Builds: []compiler.Config{
		// 		{Name: "build", Version: "3.10.4", Includes: []string{"../build-auto-ysf/dependencies/samp-stdlib"}},
		// 	},
		// }, args{"build", true}, "gamemodes/test.amx", false},
	}
	for _, tt := range tests {
		err := os.MkdirAll(filepath.Join(tt.pkg.local, "gamemodes"), 0755)
		if err != nil {
			panic(err)
		}

		err = ioutil.WriteFile(filepath.Join(tt.pkg.local, tt.pkg.Entry), tt.sourceCode, 0755)
		if err != nil {
			panic(err)
		}

		t.Run(tt.name, func(t *testing.T) {
			gotOutput, err := tt.pkg.Build(tt.args.build, tt.args.ensure)
			if (err != nil) != tt.wantErr {
				t.Errorf("Package.Build() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOutput != tt.wantOutput {
				t.Errorf("Package.Build() = %v, want %v", gotOutput, tt.wantOutput)
			}
		})
	}
}
