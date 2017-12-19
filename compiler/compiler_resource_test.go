package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/types"
)

func Test_GetCompilerPackageInfo(t *testing.T) {
	type args struct {
		os      string
		version types.CompilerVersion
	}
	tests := []struct {
		name         string
		args         args
		wantPkg      Package
		wantFilename string
		wantErr      bool
	}{
		{
			"v darwin",
			args{"darwin", "3.10.4"},
			Package{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.4/pawnc-3.10.4-darwin.zip",
				Method: download.Unzip,
				Paths: map[string]string{
					"pawnc-3.10.4-darwin/bin/pawncc":         "pawncc",
					"pawnc-3.10.4-darwin/lib/libpawnc.dylib": "libpawnc.dylib",
				},
			},
			"pawnc-3.10.4-darwin.zip",
			false,
		},
		{
			"v linux",
			args{"linux", "3.10.4"},
			Package{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.4/pawnc-3.10.4-linux.tar.gz",
				Method: download.Untar,
				Paths: map[string]string{
					"pawnc-3.10.4-linux/bin/pawncc":      "pawncc",
					"pawnc-3.10.4-linux/lib/libpawnc.so": "libpawnc.so",
				},
			},
			"pawnc-3.10.4-linux.tar.gz",
			false,
		},
		{
			"v windows",
			args{"windows", "3.10.4"},
			Package{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.4/pawnc-3.10.4-windows.zip",
				Method: download.Unzip,
				Paths: map[string]string{
					"pawnc-3.10.4-windows/bin/pawncc.exe": "pawncc.exe",
					"pawnc-3.10.4-windows/bin/pawnc.dll":  "pawnc.dll",
				},
			},
			"pawnc-3.10.4-windows.zip",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, gotFilename, gotErr := GetCompilerPackageInfo(tt.args.os, tt.args.version)
			if tt.wantErr {
				assert.Error(t, gotErr)
			} else {
				assert.NoError(t, gotErr)
			}
			assert.Equal(t, tt.wantPkg.URL, gotPkg.URL)
			// assert.Equal(t, tt.wantPkg.Method, gotPkg.Method) // function address assert does not work
			assert.Equal(t, tt.wantPkg.Paths, gotPkg.Paths)
			assert.Equal(t, tt.wantFilename, gotFilename)
		})
	}
}
