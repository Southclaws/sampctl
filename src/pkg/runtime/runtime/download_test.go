package runtime

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ServerFromNet(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"latest-alias", "latest"},
		{"exact-version", "0.3.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			cacheDir := filepath.Join(rootDir, "cache")
			dir := filepath.Join(rootDir, "server-dir")
			platform := currentTestPlatform()

			expectedBinary := seedRuntimeRemoteFixture(t, cacheDir, tt.version, platform)

			err := FromNet(cacheDir, tt.version, dir, platform)
			assert.NoError(t, err)
			assert.FileExists(t, filepath.Join(dir, expectedBinary))
		})
	}
}

func Test_ServerFromCache(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"latest-alias", "latest"},
		{"exact-version", "0.3.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			cacheDir := filepath.Join(rootDir, "cache")
			dir := filepath.Join(rootDir, "server-dir")
			platform := currentTestPlatform()

			expectedBinary := seedRuntimeCacheFixture(t, cacheDir, tt.version, platform)

			gotHit, err := FromCache(cacheDir, tt.version, dir, platform)
			assert.NoError(t, err)
			assert.True(t, gotHit)
			assert.FileExists(t, filepath.Join(dir, expectedBinary))
		})
	}
}
