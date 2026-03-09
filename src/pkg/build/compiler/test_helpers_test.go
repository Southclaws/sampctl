package compiler

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func seedCompilerCacheFixture(t *testing.T, cacheDir string, meta versioning.DependencyMeta, platform string) download.Compiler {
	t.Helper()

	manifest := offlineCompilerManifest()
	compiler, ok := manifest[platform]
	require.True(t, ok, "missing manifest for platform %s", platform)

	require.NoError(t, os.MkdirAll(cacheDir, 0o700))
	require.NoError(t, download.WriteCompilerCacheFile(cacheDir, mustJSON(t, manifest)))

	archivePath := filepath.Join(t.TempDir(), offlineCompilerArchiveName(platform, meta.Tag))
	files := offlineCompilerArchiveFiles(platform)
	createCompilerArchive(t, archivePath, compiler.Method, files)

	matcher := regexp.MustCompile(compiler.Match)
	res := infraresource.NewGitHubReleaseResource(meta, matcher, infraresource.ResourceTypeCompiler, nil)
	res.SetCacheDir(cacheDir)
	res.SetCacheTTL(0)
	res.SetLocalPath(archivePath)
	require.NoError(t, res.EnsureFromLocal(context.Background(), meta.Tag, ""))

	return compiler
}

func newFakeCompilerDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "pawncc")
	script := `#!/bin/sh
input="$1"
output=""
debug=0

for arg in "$@"
do
	case "$arg" in
		-o*) output="${arg#-o}" ;;
		-d3) debug=1 ;;
	esac
done

write_output() {
	if [ -n "$output" ]; then
		mkdir -p "$(dirname "$output")"
		: > "$output"
	fi
}

print_stats() {
	echo "Header size:             60 bytes"
	echo "Code size:              $1 bytes"
	echo "Data size:                0 bytes"
	echo "Stack/heap size:      16384 bytes; estimated max. usage=8 cells ($2 bytes)"
	echo "Total requirements:   $3 bytes"
}

case "$input" in
	*build-simple-pass/script.pwn)
		write_output
		if [ "$debug" -eq 1 ]; then
			print_stats 184 20 16628
		fi
		exit 0
		;;
	*build-local-include-pass/script.pwn)
		write_output
		print_stats 220 32 16664
		exit 0
		;;
	*build-local-include-warn/script.pwn)
		write_output
		echo "library.inc(6) : warning 203: symbol is never used: \"b\""
		echo "script.pwn(5) : warning 203: symbol is never used: \"a\""
		print_stats 276 32 16720
		exit 0
		;;
	*build-simple-fail/script.pwn)
		echo "script.pwn(1) : error 001: invalid function or declaration"
		echo "script.pwn(3) : error 001: invalid function or declaration"
		echo "script.pwn(2) : warning 203: symbol is never used: \"a\""
		echo "script.pwn(2) : error 013: no entry point (no public functions)"
		exit 1
		;;
	*build-fatal/script.pwn)
		echo "script.pwn(1) : fatal error 100: cannot read from file: \"idonotexist\""
		exit 1
		;;
esac

echo "unexpected input: $input" >&2
exit 2
`

	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "libpawnc.so"), []byte("fixture"), 0o644))

	return dir
}

func offlineCompilerManifest() download.Compilers {
	return download.Compilers{
		"linux": {
			Match:  `compiler-linux-(.*)\.tar\.gz$`,
			Method: download.ExtractTgz,
			Binary: "pawncc",
			Paths: map[string]string{
				"pawncc":      "pawncc",
				"libpawnc.so": "libpawnc.so",
			},
		},
		"darwin": {
			Match:  `compiler-darwin-(.*)\.tar\.gz$`,
			Method: download.ExtractTgz,
			Binary: "pawncc",
			Paths: map[string]string{
				"pawncc":         "pawncc",
				"libpawnc.dylib": "libpawnc.dylib",
			},
		},
		"windows": {
			Match:  `compiler-windows-(.*)\.zip$`,
			Method: download.ExtractZip,
			Binary: "pawncc.exe",
			Paths: map[string]string{
				"pawncc.exe": "pawncc.exe",
				"pawnc.dll":  "pawnc.dll",
			},
		},
	}
}

func offlineCompilerArchiveName(platform, tag string) string {
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	if platform == "windows" {
		return fmt.Sprintf("compiler-%s-%s.zip", platform, tag)
	}
	return fmt.Sprintf("compiler-%s-%s.tar.gz", platform, tag)
}

func offlineCompilerArchiveFiles(platform string) map[string]string {
	switch platform {
	case "linux":
		return map[string]string{
			"pawncc":      "#!/bin/sh\nexit 0\n",
			"libpawnc.so": "fixture",
		}
	case "darwin":
		return map[string]string{
			"pawncc":         "#!/bin/sh\nexit 0\n",
			"libpawnc.dylib": "fixture",
		}
	case "windows":
		return map[string]string{
			"pawncc.exe": "fixture",
			"pawnc.dll":  "fixture",
		}
	default:
		return map[string]string{}
	}
}

func createCompilerArchive(t *testing.T, archivePath, method string, files map[string]string) {
	t.Helper()

	switch method {
	case download.ExtractZip:
		createCompilerZipArchive(t, archivePath, files)
	case download.ExtractTgz:
		createCompilerTgzArchive(t, archivePath, files)
	default:
		t.Fatalf("unsupported archive method %s", method)
	}
}

func createCompilerZipArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	zw := zip.NewWriter(f)
	for _, name := range sortedKeys(files) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(files[name]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}

func createCompilerTgzArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	gzw := gzip.NewWriter(f)
	defer gzw.Close() //nolint:errcheck

	tw := tar.NewWriter(gzw)
	defer tw.Close() //nolint:errcheck

	for _, name := range sortedKeys(files) {
		body := []byte(files[name])
		mode := int64(0o644)
		if filepath.Base(name) == "pawncc" {
			mode = 0o755
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: mode,
			Size: int64(len(body)),
		}))
		_, err := tw.Write(body)
		require.NoError(t, err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newCompilerReleaseClient(t *testing.T, meta versioning.DependencyMeta, assetName string, assetBody []byte) *github.Client {
	t.Helper()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fmt.Sprintf("/repos/%s/%s/releases/tags/%s", meta.User, meta.Repo, meta.Tag):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":%q}]}`,
				meta.Tag,
				assetName,
				srv.URL+"/assets/"+assetName,
			)
		case "/assets/" + assetName:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(assetBody)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := github.NewClient(srv.Client())
	parsed, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)
	client.BaseURL = parsed

	return client
}

func cachedCompilerAssetPath(t *testing.T, cacheDir string, meta versioning.DependencyMeta, compiler download.Compiler) string {
	t.Helper()

	matcher := regexp.MustCompile(compiler.Match)
	res := infraresource.NewGitHubReleaseResource(meta, matcher, infraresource.ResourceTypeCompiler, nil)
	res.SetCacheDir(cacheDir)
	res.SetCacheTTL(0)
	ok, path := res.Cached(meta.Tag)
	require.True(t, ok)
	return path
}
