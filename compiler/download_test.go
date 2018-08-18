package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

func Test_CompilerFromNet(t *testing.T) {
	type args struct {
		meta     versioning.DependencyMeta
		dir      string
		platform string
		cacheDir string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"linux-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "tests/compiler-linux-v3.10.4", "linux", "tests/cache"}, false},
		{"darwin-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "tests/compiler-darwin-v3.10.4", "darwin", "tests/cache"}, false},
		{"windows-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "tests/compiler-windows-v3.10.4", "windows", "tests/cache"}, false},
		{"linux-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "tests/compiler-linux-v3.10.8", "linux", "tests/cache"}, false},
		{"darwin-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "tests/compiler-darwin-v3.10.8", "darwin", "tests/cache"}, false},
		{"windows-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "tests/compiler-windows-v3.10.8", "windows", "tests/cache"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.MkdirAll(tt.args.cacheDir, 0700)
			assert.NoError(t, err)

			_, err = FromNet(context.Background(), gh, tt.args.meta, tt.args.dir, tt.args.platform, tt.args.cacheDir)
			assert.NoError(t, err)

			switch tt.args.platform {
			case "linux":
				assert.True(t, util.Exists(filepath.Join(tt.args.dir, "pawncc")))
				assert.True(t, util.Exists(filepath.Join(tt.args.dir, "libpawnc.so")))
			case "darwin":
				assert.True(t, util.Exists(filepath.Join(tt.args.dir, "pawncc")))
				assert.True(t, util.Exists(filepath.Join(tt.args.dir, "libpawnc.dylib")))
			case "windows":
				assert.True(t, util.Exists(filepath.Join(tt.args.dir, "pawncc.exe")))
				assert.True(t, util.Exists(filepath.Join(tt.args.dir, "pawnc.dll")))
			}
		})
	}
}

func Test_CompilerFromCache(t *testing.T) {
	type args struct {
		meta     versioning.DependencyMeta
		dir      string
		platform string
		cacheDir string
	}
	tests := []struct {
		name    string
		args    args
		wantHit bool
		wantErr bool
	}{
		{"linux-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "tests/compiler-linux-v3.10.4", "linux", "tests/cache"}, true, false},
		{"darwin-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "tests/compiler-darwin-v3.10.4", "darwin", "tests/cache"}, true, false},
		{"windows-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "tests/compiler-windows-v3.10.4", "windows", "tests/cache"}, true, false},
		{"linux-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "tests/compiler-linux-v3.10.8", "linux", "tests/cache"}, true, false},
		{"darwin-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "tests/compiler-darwin-v3.10.8", "darwin", "tests/cache"}, true, false},
		{"windows-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "tests/compiler-windows-v3.10.8", "windows", "tests/cache"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.MkdirAll(tt.args.cacheDir, 0700)
			assert.NoError(t, err)

			_, gotHit, err := FromCache(tt.args.meta, tt.args.dir, tt.args.platform, tt.args.cacheDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, gotHit, tt.wantHit)
		})
	}
}
