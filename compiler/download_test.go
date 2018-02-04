package compiler

import (
	"context"
	"fmt"
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
		{"linux-v3.10.4", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.4"}, "tests/compiler-linux-v3.10.4", "linux", "tests/cache-linux-v3.10.4"}, false},
		{"darwin-v3.10.4", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.4"}, "tests/compiler-darwin-v3.10.4", "darwin", "tests/cache-darwin-v3.10.4"}, false},
		{"windows-v3.10.4", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.4"}, "tests/compiler-windows-v3.10.4", "windows", "tests/cache-windows-v3.10.4"}, false},
		// {"linux-v3.10.5", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.5"}, "tests/compiler-linux-v3.10.5", "linux", "tests/cache-linux-v3.10.5"}, false},
		// {"darwin-v3.10.5", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.5"}, "tests/compiler-darwin-v3.10.5", "darwin", "tests/cache-darwin-v3.10.5"}, false},
		// {"windows-v3.10.5", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.5"}, "tests/compiler-windows-v3.10.5", "windows", "tests/cache-windows-v3.10.5"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, err := FromNet(context.Background(), gh, tt.args.meta, tt.args.dir, tt.args.platform, tt.args.cacheDir)
			assert.NoError(t, err)

			if gotPkg != nil {
				fmt.Printf("%#v\n", *gotPkg)
			}

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
		{"linux-v3.10.4", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.4"}, "tests/compiler-linux-v3.10.4", "linux", "tests/cache-linux-v3.10.4"}, true, false},
		{"darwin-v3.10.4", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.4"}, "tests/compiler-darwin-v3.10.4", "darwin", "tests/cache-darwin-v3.10.4"}, true, false},
		{"windows-v3.10.4", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.4"}, "tests/compiler-windows-v3.10.4", "windows", "tests/cache-windows-v3.10.4"}, true, false},
		// {"linux-v3.10.5", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.5"}, "tests/compiler-linux-v3.10.5", "linux", "tests/cache-linux-v3.10.5"}, true, false},
		// {"darwin-v3.10.5", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.5"}, "tests/compiler-darwin-v3.10.5", "darwin", "tests/cache-darwin-v3.10.5"}, true, false},
		// {"windows-v3.10.5", args{versioning.DependencyMeta{User: "Zeex", Repo: "pawn", Version: "v3.10.5"}, "tests/compiler-windows-v3.10.5", "windows", "tests/cache-windows-v3.10.5"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPkg, gotHit, err := FromCache(tt.args.meta, tt.args.dir, tt.args.platform, tt.args.cacheDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if gotPkg != nil {
				fmt.Printf("%#v\n", *gotPkg)
			}
			assert.Equal(t, gotHit, tt.wantHit)
		})
	}
}
