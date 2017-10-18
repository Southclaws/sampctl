package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ServerFromNet(t *testing.T) {
	type args struct {
		endpoint    string
		cacheDir    string
		version     string
		dir         string
		getCompiler bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "latest", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3.7", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3.7-R2-2-1", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3.7-R2-1", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3z", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3z-R4", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3z-R3", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3z-R2-2", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3z-R1", "./testspace", false}, false},
		// {"valid", args{"http://files.sa-mp.com", "./testcache", "0.3z-R1-2", "./testspace", false}, false},

		{"valid", args{"http://files.sa-mp.com", "./testcache", "latest", "./testspace", true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ServerFromNet(tt.args.endpoint, tt.args.cacheDir, tt.args.version, tt.args.dir, tt.args.getCompiler)
			assert.NoError(t, err)
		})
	}
}

func Test_ServerFromCache(t *testing.T) {
	type args struct {
		cacheDir    string
		version     string
		dir         string
		getCompiler bool
	}
	tests := []struct {
		name    string
		args    args
		wantHit bool
		wantErr bool
	}{
		{"valid", args{"./testcache", "0.4a-RC1", "./testspace", false}, false, true},
		{"valid", args{"./testcache", "latest", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3.7", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3.7-R2-2-1", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3.7-R2-1", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3z", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3z-R4", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3z-R3", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3z-R2-2", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3z-R1", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "0.3z-R1-2", "./testspace", false}, true, false},
		{"valid", args{"./testcache", "latest", "./testspace", true}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHit, err := ServerFromCache(tt.args.cacheDir, tt.args.version, tt.args.dir, tt.args.getCompiler)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, gotHit, tt.wantHit)
		})
	}
}
