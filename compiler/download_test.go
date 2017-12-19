package compiler

import (
	"runtime"
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
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{"tests/cache", "3.10.4", "tests/compiler"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FromNet(tt.args.cacheDir, tt.args.version, tt.args.dir, runtime.GOOS)
			assert.NoError(t, err)

			// assumes the tests are being run in linux/darwin (sorry!)
			assert.True(t, util.Exists("./tests/compiler/pawncc"))
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
		name    string
		args    args
		wantHit bool
		wantErr bool
	}{
		{"valid", args{"./tests/cache", "3.10.4", "./tests/compiler"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHit, err := FromCache(tt.args.cacheDir, tt.args.version, tt.args.dir, runtime.GOOS)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, gotHit, tt.wantHit)
		})
	}
}
