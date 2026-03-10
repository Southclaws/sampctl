package runtime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	res "github.com/Southclaws/sampctl/src/resource"
)

func seedRuntimeCacheFixture(t *testing.T, cacheDir, requestedVersion, platform string) string {
	t.Helper()

	archiveName := offlineRuntimeArchiveName(platform, requestedVersion)
	archivePath := filepath.Join(t.TempDir(), archiveName)
	archiveBytes := createRuntimeArchive(t, archivePath, platform, map[string]string{
		expectedRuntimeBinary(platform): "fixture",
	})
	checksum := md5.Sum(archiveBytes)

	writeRuntimeFixtureManifest(t, cacheDir, offlineRuntimeURL("linux", "0.3.7"), offlineRuntimeURL("windows", "0.3.7"), hex.EncodeToString(checksum[:]), hex.EncodeToString(checksum[:]))
	seedCachedRuntimeArchive(t, cacheDir, requestedVersion, offlineRuntimeURL(platform, "0.3.7"), archivePath)

	return expectedRuntimeBinary(platform)
}

func seedRuntimeRemoteFixture(t *testing.T, cacheDir, requestedVersion, platform string) string {
	t.Helper()

	archiveName := offlineRuntimeArchiveName(platform, requestedVersion)
	archivePath := filepath.Join(t.TempDir(), archiveName)
	archiveBytes := createRuntimeArchive(t, archivePath, platform, map[string]string{
		expectedRuntimeBinary(platform): "fixture",
	})
	checksum := md5.Sum(archiveBytes)
	assetBody, err := os.ReadFile(archivePath)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assets/"+archiveName {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(assetBody)
	}))
	t.Cleanup(server.Close)

	linuxURL := offlineRuntimeURL("linux", "0.3.7")
	windowsURL := offlineRuntimeURL("windows", "0.3.7")
	if platform == "windows" {
		windowsURL = server.URL + "/assets/" + archiveName
	} else {
		linuxURL = server.URL + "/assets/" + archiveName
	}

	writeRuntimeFixtureManifest(t, cacheDir, linuxURL, windowsURL, hex.EncodeToString(checksum[:]), hex.EncodeToString(checksum[:]))

	return expectedRuntimeBinary(platform)
}

func writeRuntimeFixtureManifest(t *testing.T, cacheDir, linuxURL, windowsURL, linuxChecksum, windowsChecksum string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(cacheDir, 0o700))
	data, err := json.Marshal(download.Runtimes{
		Aliases: map[string]string{"latest": "0.3.7"},
		Packages: []download.RuntimePackage{{
			Version:       "0.3.7",
			Linux:         linuxURL,
			Win32:         windowsURL,
			LinuxChecksum: linuxChecksum,
			Win32Checksum: windowsChecksum,
			LinuxPaths:    map[string]string{"samp03svr": "samp03svr"},
			Win32Paths:    map[string]string{"samp-server.exe": "samp-server.exe"},
		}},
	})
	require.NoError(t, err)
	require.NoError(t, download.WriteRuntimeCacheFile(cacheDir, data))
}

func seedCachedRuntimeArchive(t *testing.T, cacheDir, version, rawURL, archivePath string) {
	t.Helper()

	hr, err := infraresource.NewHTTPFileResource(rawURL, version, infraresource.ResourceTypeServerBinary)
	require.NoError(t, err)
	hr.SetCacheDir(cacheDir)
	hr.SetCacheTTL(0)
	hr.SetLocalPath(archivePath)
	require.NoError(t, hr.EnsureFromLocal(context.Background(), version, ""))
}

func seedCachedPluginPackage(t *testing.T, cacheDir string, meta versioning.DependencyMeta, pkg pawnpackage.Package, archiveName string, files map[string]string) {
	t.Helper()

	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o700))
	data, err := json.Marshal(pkg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), data, 0o644))

	archivePath := filepath.Join(cachePath, archiveName)
	if filepath.Ext(archiveName) == ".zip" {
		createRuntimeZipArchive(t, archivePath, files)
	} else {
		createRuntimeTgzArchive(t, archivePath, files)
	}
}

func pluginFixturePackage(meta versioning.DependencyMeta, resources []res.Resource) pawnpackage.Package {
	return pawnpackage.Package{
		DependencyMeta: meta,
		Resources:      resources,
		Runtime:        &run.Runtime{Plugins: []run.Plugin{run.Plugin(meta.Repo)}},
	}
}

func offlineRuntimeURL(platform, version string) string {
	return fmt.Sprintf("https://fixtures.example/%s/%s", version, offlineRuntimeArchiveName(platform, version))
}

func offlineRuntimeArchiveName(platform, version string) string {
	if platform == "windows" {
		return fmt.Sprintf("samp-server-%s.zip", version)
	}
	return fmt.Sprintf("samp-server-%s.tar.gz", version)
}

func createRuntimeArchive(t *testing.T, archivePath, platform string, files map[string]string) []byte {
	t.Helper()

	if platform == "windows" {
		createRuntimeZipArchive(t, archivePath, files)
	} else {
		createRuntimeTgzArchive(t, archivePath, files)
	}

	data, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	return data
}

func expectedRuntimeBinary(platform string) string {
	if run.DetectRuntimeType("0.3.7") == run.RuntimeTypeOpenMP {
		return "omp-server"
	}
	if platform == "windows" {
		return "samp-server.exe"
	}
	return "samp03svr"
}

func createRuntimeZipArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	zw := zip.NewWriter(f)
	for _, name := range sortedRuntimeKeys(files) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(files[name]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}

func createRuntimeTgzArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	gzw := gzip.NewWriter(f)
	defer gzw.Close() //nolint:errcheck

	tw := tar.NewWriter(gzw)
	defer tw.Close() //nolint:errcheck

	for _, name := range sortedRuntimeKeys(files) {
		body := []byte(files[name])
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}))
		_, err := tw.Write(body)
		require.NoError(t, err)
	}
}

func sortedRuntimeKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func currentTestPlatform() string {
	if runtime.GOOS == "darwin" {
		return "linux"
	}
	return runtime.GOOS
}
