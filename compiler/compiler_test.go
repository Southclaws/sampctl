package compiler

import (
	"testing"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/util"
	"github.com/stretchr/testify/assert"
)

func Test_GetCompilerPackageInfo(t *testing.T) {
	type args struct {
		os      string
		version Version
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

func Test_CompilerFromNet(t *testing.T) {
	type args struct {
		cacheDir string
		version  Version
		dir      string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{"tests/cache", "3.10.4", "tests/compiler"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FromNet(tt.args.cacheDir, tt.args.version, tt.args.dir)
			assert.NoError(t, err)

			// assumes the tests are being run in linux/darwin (sorry!)
			assert.True(t, util.Exists("./tests/compiler/pawncc"))
		})
	}
}

func Test_CompilerFromCache(t *testing.T) {
	type args struct {
		cacheDir string
		version  Version
		dir      string
	}
	tests := []struct {
		name    string
		args    args
		wantHit bool
		wantErr bool
	}{
		{"valid", args{"./tests/cache", "3.10.4", "./tests/compiler"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHit, err := FromCache(tt.args.cacheDir, tt.args.version, tt.args.dir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, gotHit, tt.wantHit)
		})
	}
}

func TestCompileSource(t *testing.T) {
	type args struct {
		cacheDir string
		config   Config
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{
			util.FullPath("./tests/cache"),
			Config{
				WorkingDir: ".",
				Input:      "./tests/compile/compile_test.pwn",
				Output:     "./tests/compile/compile_test.amx",
				Includes:   []string{},
				Version:    "3.10.4",
			}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CompileSource(".", tt.args.cacheDir, tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("CompileSource() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.True(t, util.Exists("./tests/compile/compile_test.amx"))
		})
	}
}
