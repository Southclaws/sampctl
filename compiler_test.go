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
				Paths:  []string{"pawnc-3.10.2-darwin/bin/pawncc", "pawnc-3.10.2-darwin/lib/libpawnc.dylib"},
			},
			"pawnc-3.10.2-darwin.zip",
			false,
		},
		{
			"v windows",
			args{"windows", "3.10.2"},
			CompilerPackage{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-windows.zip",
				Method: Unzip,
				Paths:  []string{"pawnc-3.10.2-windows/bin/pawncc.exe", "pawnc-3.10.2-windows/bin/pawnc.dll"},
			},
			"pawnc-3.10.2-windows.zip",
			false,
		},
		{
			"v linux",
			args{"linux", "3.10.2"},
			CompilerPackage{
				URL:    "https://github.com/Zeex/pawn/releases/download/v3.10.2/pawnc-3.10.2-linux.tar.gz",
				Method: Untar,
				Paths:  []string{"pawnc-3.10.2-linux/bin/pawncc", "pawnc-3.10.2-linux/lib/libpawnc.so"},
			},
			"pawnc-3.10.2-linux.tar.gz",
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
			assert.Equal(t, gotPkg.URL, tt.wantPkg.URL)
			// assert.Equal(t, gotPkg.Method, tt.wantPkg.Method) // function address assert does not work
			assert.Equal(t, gotPkg.Paths, tt.wantPkg.Paths)
			assert.Equal(t, gotFilename, tt.wantFilename)
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
		{"valid", args{"./testcache", "3.10.2", "./testcompiler"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompilerFromNet(tt.args.cacheDir, tt.args.version, tt.args.dir)
			assert.NoError(t, err)
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
		{"valid", args{"./testcache", "3.10.2", "./testcompiler"}, true, false},
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
