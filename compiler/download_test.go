package compiler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

func Test_CompilerFromNet(t *testing.T) {
	type args struct {
		cacheDir string
		version  types.CompilerVersion
		dir      string
	}
	tests := []struct {
		name     string
		args     args
		platform string
		wantErr  bool
	}{
		{"valid", args{"tests/cache", "3.10.4", "tests/compiler"}, "linux", false},
		{"valid", args{"tests/cache", "3.10.4", "tests/compiler"}, "darwin", false},
		{"valid", args{"tests/cache", "3.10.4", "tests/compiler"}, "windows", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FromNet(tt.args.cacheDir, tt.args.version, tt.args.dir, tt.platform)
			assert.NoError(t, err)

			switch tt.platform {
			case "linux":
				assert.True(t, util.Exists("./tests/compiler/pawncc"))
				assert.True(t, util.Exists("./tests/compiler/libpawnc.so"))
			case "darwin":
				assert.True(t, util.Exists("./tests/compiler/pawncc"))
				assert.True(t, util.Exists("./tests/compiler/libpawnc.dylib"))
			case "windows":
				assert.True(t, util.Exists("./tests/compiler/pawncc.exe"))
				assert.True(t, util.Exists("./tests/compiler/pawnc.dll"))
			}
		})
	}
}

func Test_CompilerFromCache(t *testing.T) {
	type args struct {
		cacheDir string
		version  types.CompilerVersion
		dir      string
	}
	tests := []struct {
		name     string
		args     args
		platform string
		wantHit  bool
		wantErr  bool
	}{
		{"valid", args{"./tests/cache", "3.10.4", "./tests/compiler"}, "linux", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHit, err := FromCache(tt.args.cacheDir, tt.args.version, tt.args.dir, tt.platform)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, gotHit, tt.wantHit)
		})
	}
}
