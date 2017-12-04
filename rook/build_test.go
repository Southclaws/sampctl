package rook

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/compiler"
	"github.com/Southclaws/sampctl/util"
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
		{"stdlib", []byte(`#include <a_samp>
			main() {print("hi");}
			`), Package{
			local:  util.FullPath("./tests/build-auto-stdlib"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
			},
			Builds: []compiler.Config{
				{Name: "build", Version: "3.10.4"},
			},
		}, args{"build", true}, "gamemodes/test.amx", false},
		{"sif", []byte(`#include <SIF/Item.pwn>
			main() {DefineItemType("name[]", "uname[]", 1, 1);}
			`), Package{
			local:  util.FullPath("./tests/build-auto-sif"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
				"Southclaws/SIF:1.6.2",
				"Misiur/YSI-Includes",
				"samp-incognito/samp-streamer-plugin:2.9.1",
				"Zeex/amx_assembly",
			},
			Builds: []compiler.Config{
				{Name: "build", Version: "3.10.4"},
			},
		}, args{"build", true}, "gamemodes/test.amx", false},
		{"ysf", []byte(`#include <a_samp>
			#include <YSF>
			main() {}
			`), Package{
			local:  util.FullPath("./tests/build-auto-ysf"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
				"kurta999/YSF/sampsvr_files/pawno/include",
			},
			Builds: []compiler.Config{
				{Name: "build", Version: "3.10.4"},
			},
		}, args{"build", true}, "gamemodes/test.amx", false},
		{"spaces", []byte(`#include <a_samp>
			main() {}
			`), Package{
			local:  util.FullPath("./tests/build-auto- spaces"),
			Entry:  "gamemodes/test.pwn",
			Output: "gamemodes/test.amx",
			Dependencies: []DependencyString{
				"Southclaws/samp-stdlib:0.3.7-R2-2-1",
			},
			Builds: []compiler.Config{
				{Name: "build", Version: "3.10.4"},
			},
		}, args{"build", true}, "gamemodes/test.amx", false},
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
