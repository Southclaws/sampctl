package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetCompilerPackageInfo(t *testing.T) {
	type args struct {
		os      string
		version string
	}
	tests := []struct {
		name         string
		args         args
		wantPkg      CompilerPackage
		wantFilename string
		wantErr      bool
	}{
		{
			"v darwin",
			args{"darwin", "3.10.2"},
			CompilerPackage{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-darwin.zip",
				Method: Unzip,
				Paths: map[string]string{
					"pawnc-3.10.2-darwin/bin/pawncc":         "pawncc",
					"pawnc-3.10.2-darwin/lib/libpawnc.dylib": "libpawnc.dylib",
				},
			},
			"pawnc-3.10.2-darwin.zip",
			false,
		},
		{
			"v linux",
			args{"linux", "3.10.2"},
			CompilerPackage{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-linux.tar.gz",
				Method: Untar,
				Paths: map[string]string{
					"pawnc-3.10.2-linux/bin/pawncc":      "pawncc",
					"pawnc-3.10.2-linux/lib/libpawnc.so": "libpawnc.so",
				},
			},
			"pawnc-3.10.2-linux.tar.gz",
			false,
		},
		{
			"v windows",
			args{"windows", "3.10.2"},
			CompilerPackage{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-windows.zip",
				Method: Unzip,
				Paths: map[string]string{
					"pawnc-3.10.2-windows/bin/pawncc.exe": "pawncc.exe",
					"pawnc-3.10.2-windows/bin/pawnc.dll":  "pawnc.dll",
				},
			},
			"pawnc-3.10.2-windows.zip",
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
		version  string
		dir      string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{"./tests/cache", "3.10.2", "./tests/compiler"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompilerFromNet(tt.args.cacheDir, tt.args.version, tt.args.dir)
			assert.NoError(t, err)

			// assumes the tests are being run in linux/darwin (sorry!)
			assert.True(t, exists("./tests/compiler/pawncc"))
		})
	}
}

func Test_CompilerFromCache(t *testing.T) {
	type args struct {
		cacheDir string
		version  string
		dir      string
	}
	tests := []struct {
		name    string
		args    args
		wantHit bool
		wantErr bool
	}{
		{"valid", args{"./tests/cache", "3.10.2", "./tests/compiler"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHit, err := CompilerFromCache(tt.args.cacheDir, tt.args.version, tt.args.dir)
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
		input    string
		output   string
		cacheDir string
		version  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{"./tests/compile/compile_test.pwn", "./tests/compile/compile_test.amx", fullPath("./tests/cache"), "3.10.2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CompileSource(tt.args.input, tt.args.output, tt.args.cacheDir, tt.args.version); (err != nil) != tt.wantErr {
				t.Errorf("CompileSource() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.True(t, exists("./tests/compile/compile_test.amx"))
		})
	}
}
