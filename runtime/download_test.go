package runtime

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ServerFromNet(t *testing.T) {
	type args struct {
		endpoint string
		cacheDir string
		version  string
		dir      string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "latest", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3.7", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3.7-R2-2-1", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3.7-R2-1", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3z", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3z-R4", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3z-R3", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3z-R2-2", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3z-R1", "./tests/server-dir"}, false},
		{"valid", args{"http://files.sa-mp.com", "./tests/cache", "0.3z-R1-2", "./tests/server-dir"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FromNet(tt.args.endpoint, tt.args.cacheDir, tt.args.version, tt.args.dir, runtime.GOOS)
			assert.NoError(t, err)
		})
	}
}

// Note: this test assumes the previous test has run and the packages are already cached

func Test_ServerFromCache(t *testing.T) {
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
		{"valid", args{"./tests/cache", "0.4a-RC1", "./tests/server-dir"}, false, true},
		{"valid", args{"./tests/cache", "latest", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3.7", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3.7-R2-2-1", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3.7-R2-1", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3z", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3z-R4", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3z-R3", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3z-R2-2", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3z-R1", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "0.3z-R1-2", "./tests/server-dir"}, true, false},
		{"valid", args{"./tests/cache", "latest", "./tests/server-dir"}, true, false},
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
