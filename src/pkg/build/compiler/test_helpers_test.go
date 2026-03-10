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
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

var compilerFixtureOnce sync.Once

func ensureCompilerSourceFixtures(t *testing.T) {
	t.Helper()

	compilerFixtureOnce.Do(func() {
		fixtures := map[string]map[string]string{
			"build-simple-pass": {
				"script.pwn": "main() {}\n",
			},
			"build-simple-fail": {
				"script.pwn": "broken\n",
			},
			"build-local-include-pass": {
				"script.pwn":  `#include "library"` + "\nmain() {}\n",
				"library.inc": "stock lib() { return 1; }\n",
			},
			"build-local-include-warn": {
				"script.pwn":  `#include "library"` + "\nmain() {}\n",
				"library.inc": "stock lib() { new b; return b; }\n",
			},
			"build-fatal": {
				"script.pwn": `#include "idonotexist"` + "\nmain() {}\n",
			},
		}

		for dir, files := range fixtures {
			base := filepath.Join("tests", dir)
			for name, body := range files {
				path := filepath.Join(base, name)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read fixture file %s: %v", path, err)
				}
				if string(data) != body {
					t.Fatalf("fixture file %s has unexpected contents", path)
				}
			}
		}
	})
}

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
	binaryPath := filepath.Join(dir, fakeCompilerBinaryName())
	sourcePath := filepath.Join(dir, "fake_pawncc.go")
	source := `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeOutput(output string) error {
	if output == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	return os.WriteFile(output, nil, 0o644)
}

func printStats(code, estimate, total int) {
	fmt.Printf("Header size:             60 bytes\n")
	fmt.Printf("Code size:              %d bytes\n", code)
	fmt.Printf("Data size:                0 bytes\n")
	fmt.Printf("Stack/heap size:      16384 bytes; estimated max. usage=8 cells (%d bytes)\n", estimate)
	fmt.Printf("Total requirements:   %d bytes\n", total)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing input")
		os.Exit(2)
	}

	input := filepath.ToSlash(args[0])
	output := ""
	debug := false
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "-o"):
			output = arg[2:]
		case arg == "-d3":
			debug = true
		}
	}

	if strings.Contains(input, "/build-simple-pass/script.pwn") {
		if err := writeOutput(output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if debug {
			printStats(184, 20, 16628)
		}
		return
	}

	if strings.Contains(input, "/build-local-include-pass/script.pwn") {
		if err := writeOutput(output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		printStats(220, 32, 16664)
		return
	}

	if strings.Contains(input, "/build-local-include-warn/script.pwn") {
		if err := writeOutput(output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("library.inc(6) : warning 203: symbol is never used: \"b\"")
		fmt.Println("script.pwn(5) : warning 203: symbol is never used: \"a\"")
		printStats(276, 32, 16720)
		return
	}

	if strings.Contains(input, "/build-simple-fail/script.pwn") {
		fmt.Println("script.pwn(1) : error 001: invalid function or declaration")
		fmt.Println("script.pwn(3) : error 001: invalid function or declaration")
		fmt.Println("script.pwn(2) : warning 203: symbol is never used: \"a\"")
		fmt.Println("script.pwn(2) : error 013: no entry point (no public functions)")
		os.Exit(1)
	}

	if strings.Contains(input, "/build-fatal/script.pwn") {
		fmt.Println("script.pwn(1) : fatal error 100: cannot read from file: \"idonotexist\"")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "unexpected input: %s\n", input)
	os.Exit(2)
}
`

	require.NoError(t, os.WriteFile(sourcePath, []byte(source), 0o644))
	cmd := exec.Command("go", "build", "-o", binaryPath, sourcePath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, os.WriteFile(filepath.Join(dir, fakeCompilerLibraryName()), []byte("fixture"), 0o644))

	return dir
}

func fakeCompilerBinaryName() string {
	if runtime.GOOS == "windows" {
		return "pawncc.exe"
	}
	return "pawncc"
}

func fakeCompilerLibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libpawnc.dylib"
	case "windows":
		return "pawnc.dll"
	default:
		return "libpawnc.so"
	}
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
