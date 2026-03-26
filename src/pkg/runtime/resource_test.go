package runtime

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
)

func TestGetServerBinary(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	data, err := json.Marshal(download.Runtimes{
		Aliases: map[string]string{"latest": "0.3.7"},
		Packages: []download.RuntimePackage{
			{
				Version:    "0.3.7",
				LinuxPaths: map[string]string{"samp03svr": "bin/samp03svr"},
				Win32Paths: map[string]string{"samp-server.exe": "bin/samp-server.exe"},
			},
			{
				Version:    "v1.0.0-openmp",
				LinuxPaths: map[string]string{"omp-server": "samp03svr"},
				Win32Paths: map[string]string{"omp-server.exe": "samp-server.exe"},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, download.WriteRuntimeCacheFile(cacheDir, data))

	assert.Equal(t, filepath.Join("bin", "samp03svr"), getServerBinary(cacheDir, "latest", "linux"))
	assert.Equal(t, filepath.Join("bin", "samp-server.exe"), getServerBinary(cacheDir, "0.3.7", "windows"))
	assert.Equal(t, "omp-server", getServerBinary(cacheDir, "openmp", "linux"))
	assert.Equal(t, "omp-server.exe", getServerBinary(cacheDir, "openmp", "windows"))
	assert.Equal(t, "omp-server", getServerBinary(cacheDir, "v1.0.0-openmp", "linux"))
	assert.Equal(t, "omp-server.exe", getServerBinary(cacheDir, "v1.0.0-openmp", "windows"))
	assert.Empty(t, getServerBinary(cacheDir, "0.3.7", "plan9"))
}
