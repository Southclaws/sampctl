package resource

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestBaseResourceEnsureFromLocalAndCached(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "source.txt")
	require.NoError(t, os.WriteFile(source, []byte("local-data"), 0o644))

	br := NewBaseResource("fixture/local", "v1", ResourceTypeArbitraryFile)
	br.SetCacheDir(cacheDir)
	br.SetCacheTTL(0)
	br.SetLocalPath(source)

	target := filepath.Join(t.TempDir(), "target.txt")
	require.NoError(t, br.EnsureFromLocal(context.Background(), "v1", target))

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "local-data", string(data))

	cached, cachedPath := br.Cached("v1")
	assert.True(t, cached)
	assert.FileExists(t, cachedPath)

	require.NoError(t, os.WriteFile(source, []byte("changed"), 0o644))
	target2 := filepath.Join(t.TempDir(), "target2.txt")
	require.NoError(t, br.EnsureFromLocal(context.Background(), "v1", target2))

	data, err = os.ReadFile(target2)
	require.NoError(t, err)
	assert.NotEmpty(t, string(data))
}

func TestBaseResourceEnsureFromLocalRequiresPath(t *testing.T) {
	t.Parallel()

	br := NewBaseResource("fixture/local", "v1", ResourceTypeArbitraryFile)
	br.SetCacheDir(t.TempDir())
	err := br.EnsureFromLocal(context.Background(), "v1", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no local path")
}

func TestBaseResourceEnsureFromURLUsesCache(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("remote-data"))
	}))
	defer server.Close()

	br := NewBaseResource("fixture/url", "v1", ResourceTypeArbitraryFile)
	br.SetCacheDir(t.TempDir())
	br.SetCacheTTL(0)
	br.SetDownloadURL(server.URL + "/asset.txt")

	target1 := filepath.Join(t.TempDir(), "download1.txt")
	require.NoError(t, br.EnsureFromURL(context.Background(), "v1", target1))
	assert.Equal(t, 1, requests)

	target2 := filepath.Join(t.TempDir(), "download2.txt")
	require.NoError(t, br.EnsureFromURL(context.Background(), "v1", target2))
	assert.Equal(t, 1, requests)

	data, err := os.ReadFile(target2)
	require.NoError(t, err)
	assert.Equal(t, "remote-data", string(data))
	assert.Equal(t, server.URL+"/asset.txt", br.GetDownloadURL())
}

func TestBaseResourceGetLocalPath(t *testing.T) {
	t.Parallel()

	br := NewBaseResource("fixture/local", "v1", ResourceTypeArbitraryFile)
	br.SetLocalPath("/tmp/local.txt")
	assert.Equal(t, "/tmp/local.txt", br.GetLocalPath())
}

func TestEnsureCacheDirReplacesLegacyFile(t *testing.T) {
	t.Parallel()

	cachePath := filepath.Join(t.TempDir(), "cache-entry")
	require.NoError(t, os.WriteFile(cachePath, []byte("legacy"), 0o644))

	require.NoError(t, ensureCacheDir(cachePath))
	assert.DirExists(t, cachePath)
}

func TestHTTPFileResourceLifecycle(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("http-file"))
	}))
	defer server.Close()

	hr, err := NewHTTPFileResource(server.URL+"/archive.tar.gz", "v1", ResourceTypePlugin)
	require.NoError(t, err)
	hr.SetCacheDir(t.TempDir())
	hr.SetCacheTTL(0)

	target := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, hr.Ensure(context.Background(), "v1", target))
	assert.Equal(t, 1, requests)
	assert.Equal(t, "archive.tar.gz", hr.GetFilename())
	assert.Equal(t, server.URL+"/archive.tar.gz", hr.GetURL())

	cached, cachedPath := hr.Cached("v1")
	assert.True(t, cached)
	assert.True(t, regexp.MustCompile(`archive\.tar\.gz$`).MatchString(cachedPath))

	target2 := filepath.Join(t.TempDir(), "copy.tar.gz")
	require.NoError(t, hr.Ensure(context.Background(), "v1", target2))
	assert.Equal(t, 1, requests)
	assert.Contains(t, hr.Identifier(), filepath.Join("http", "127.0.0.1"))
	assert.Contains(t, hr.Identifier(), "archive.tar.gz")
	assert.Equal(t, ResourceTypePlugin, hr.Type())
	assert.Equal(t, "v1", hr.Version())
	assert.NotEmpty(t, hr.cacheKey("v1"))
}

func TestNewHTTPFileResourceRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := NewHTTPFileResource("://bad url", "v1", ResourceTypeArbitraryFile)
	require.Error(t, err)
}

func TestGitHubReleaseResourceCachedAndGetters(t *testing.T) {
	t.Parallel()

	meta := versioning.DependencyMeta{Site: "github.com", User: "u", Repo: "r", Tag: "v1.2.3"}
	matcher := regexp.MustCompile(`asset.*\.zip`)
	res := NewGitHubReleaseResource(meta, matcher, ResourceTypePlugin, github.NewClient(nil))
	res.SetCacheDir(t.TempDir())
	res.SetCacheTTL(0)

	cacheDir := res.getCachePath(meta.Tag)
	require.NoError(t, fs.EnsureDir(cacheDir, fs.PermDirPrivate))
	legacyPath := filepath.Join(cacheDir, "asset-test.zip")
	require.NoError(t, os.WriteFile(legacyPath, []byte("zip"), 0o644))

	cached, path := res.Cached(meta.Tag)
	assert.True(t, cached)
	assert.Equal(t, legacyPath, path)
	assert.Equal(t, meta, res.GetDependencyMeta())
	assert.Equal(t, matcher.String(), res.GetAssetPattern().String())
	res.SetExtractFunc(nil)
	res.SetExtractPaths(map[string]string{"a": "b"})
}

func TestHTTPResourceEnsure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http-resource"))
	}))
	defer server.Close()

	hr := NewHTTPResource(server.URL+"/file.txt", "file.txt", "v1", ResourceTypeArbitraryFile)
	hr.SetCacheDir(t.TempDir())
	hr.SetCacheTTL(0)

	target := filepath.Join(t.TempDir(), "file.txt")
	require.NoError(t, hr.Ensure(context.Background(), "v1", target))
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "http-resource", string(data))
	assert.Equal(t, server.URL+"/file.txt", hr.GetURL())
	assert.Equal(t, "file.txt", hr.GetFilename())

	var broken HTTPResource
	err = broken.Ensure(context.Background(), "", filepath.Join(t.TempDir(), "x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no BaseResource")
}

func TestGitResourceCachedEnsureAndCopy(t *testing.T) {
	t.Parallel()

	meta := versioning.DependencyMeta{User: "u", Repo: "r", Tag: "v1.0.0"}
	gr := NewGitResource(meta, ResourceTypePawnLibrary)
	gr.SetCacheDir(t.TempDir())
	gr.SetCacheTTL(0)

	cachePath := gr.getCachePath(meta.Tag)
	require.NoError(t, os.MkdirAll(filepath.Join(cachePath, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "sub", "file.txt"), []byte("git-data"), 0o644))

	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, gr.Ensure(context.Background(), meta.Tag, target))
	data, err := os.ReadFile(filepath.Join(target, "sub", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "git-data", string(data))
	assert.Equal(t, meta, gr.GetDependencyMeta())

	copyTarget := filepath.Join(t.TempDir(), "copy")
	require.NoError(t, gr.copyToTarget(cachePath, copyTarget))
	assert.FileExists(t, filepath.Join(copyTarget, "sub", "file.txt"))
}

func TestGitHubReleaseResourceEnsure(t *testing.T) {
	meta := versioning.DependencyMeta{Site: "github.com", User: "u", Repo: "r"}
	matcher := regexp.MustCompile(`asset\.zip`)

	t.Run("requires github client", func(t *testing.T) {
		res := NewGitHubReleaseResource(meta, matcher, ResourceTypePlugin, nil)
		res.SetCacheDir(t.TempDir())
		err := res.Ensure(context.Background(), "latest", filepath.Join(t.TempDir(), "out"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no GitHub client")
	})

	t.Run("downloads extracts and copies", func(t *testing.T) {
		archive := createZipBytes(t, map[string]string{"pkg/plugin.dll": "dll"})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/u/r/releases":
				_, _ = w.Write([]byte(`[{"tag_name":"v2.0.0","assets":[{"id":7,"name":"asset.zip","browser_download_url":"https://example.invalid/asset.zip"}]}]`))
			case "/repos/u/r/releases/assets/7":
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(archive)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		client := github.NewClient(server.Client())
		baseURL := server.URL + "/"
		u, err := url.Parse(baseURL)
		require.NoError(t, err)
		client.BaseURL = u
		client.UploadURL = u

		res := NewGitHubReleaseResource(meta, matcher, ResourceTypePlugin, client)
		res.SetCacheDir(t.TempDir())
		res.SetCacheTTL(0)
		res.SetExtractFunc(download.Unzip)
		res.SetExtractPaths(map[string]string{`pkg/plugin\.dll`: "plugins/"})

		target := filepath.Join(t.TempDir(), "out")
		require.NoError(t, os.MkdirAll(filepath.Join(target, "plugins"), 0o755))
		require.NoError(t, res.Ensure(context.Background(), "latest", target))
		assert.FileExists(t, filepath.Join(target, "plugins", "plugin.dll"))
		assert.Equal(t, "v2.0.0", res.Version())
	})

	t.Run("copies cached file", func(t *testing.T) {
		serverClient := github.NewClient(nil)
		res := NewGitHubReleaseResource(versioning.DependencyMeta{Site: "github.com", User: "u", Repo: "r", Tag: "v1.0.0"}, matcher, ResourceTypePlugin, serverClient)
		res.SetCacheDir(t.TempDir())
		cacheDir := res.getCachePath("v1.0.0")
		require.NoError(t, fs.EnsureDir(cacheDir, fs.PermDirPrivate))
		cachedFile := filepath.Join(cacheDir, "asset.zip")
		require.NoError(t, os.WriteFile(cachedFile, []byte("cached"), 0o644))

		target := filepath.Join(t.TempDir(), "copied.zip")
		require.NoError(t, res.Ensure(context.Background(), "v1.0.0", target))
		data, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "cached", string(data))

		copyTarget := filepath.Join(t.TempDir(), "copy2.zip")
		require.NoError(t, res.copyToTarget(target, copyTarget))
		assert.FileExists(t, copyTarget)
	})
}

func TestFactoryFromDependencyStringAndLocalResource(t *testing.T) {
	t.Parallel()

	factory := NewDefaultResourceFactory(nil)
	res, err := factory.FromDependencyString("plugin://local/plugins/test", ResourceTypePlugin)
	require.NoError(t, err)
	_, ok := res.(*LocalResource)
	assert.True(t, ok)

	res, err = factory.FromDependencyString("Southclaws/pawn-errors", ResourceTypePawnLibrary)
	require.NoError(t, err)
	_, ok = res.(*GitResource)
	assert.True(t, ok)

	_, err = factory.FromDependencyString("not valid !!!", ResourceTypePlugin)
	require.Error(t, err)

	localFile := filepath.Join(t.TempDir(), "local.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("local"), 0o644))
	lr := NewLocalResource(localFile, ResourceTypeArbitraryFile)
	assert.Equal(t, localFile, lr.GetLocalPath())
	require.NoError(t, lr.Ensure(context.Background(), "", ""))

	target := filepath.Join(t.TempDir(), "copied.txt")
	require.NoError(t, lr.Ensure(context.Background(), "", target))
	assert.FileExists(t, target)
}

func TestDefaultResourceFactoryGitHubReleaseURL(t *testing.T) {
	t.Parallel()

	factory := NewDefaultResourceFactory(github.NewClient(nil))
	resource, err := factory.FromURL("https://github.com/user/repo/releases/download/v1.0.0/asset.zip", ResourceTypePlugin)
	require.NoError(t, err)

	ghr, ok := resource.(*GitHubReleaseResource)
	require.True(t, ok)
	assert.Equal(t, "github.com/user/repo", identifierFromDependencyMeta(ghr.GetDependencyMeta()))
}

func TestDefaultResourceManagerEnsureAllAndCleanCache(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	factory := NewDefaultResourceFactory(nil)
	manager := NewDefaultResourceManager(factory)

	localFile := filepath.Join(t.TempDir(), "resource.txt")
	require.NoError(t, os.WriteFile(localFile, []byte("content"), 0o644))
	manager.AddResource(NewLocalResource(localFile, ResourceTypeArbitraryFile))

	require.NoError(t, manager.EnsureAll(context.Background(), []Resource{NewLocalResource(localFile, ResourceTypeArbitraryFile)}))

	configDir, err := fs.ConfigDir()
	require.NoError(t, err)
	stale := filepath.Join(configDir, string(ResourceTypePlugin), "res1", "v1", "hash1")
	fresh := filepath.Join(configDir, string(ResourceTypePlugin), "res2", "v1", "hash1")
	require.NoError(t, os.MkdirAll(stale, 0o755))
	require.NoError(t, os.MkdirAll(fresh, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stale, "file.txt"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fresh, "file.txt"), []byte("new"), 0o644))

	old := time.Now().Add(-8 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))
	require.NoError(t, os.Chtimes(filepath.Join(stale, "file.txt"), old, old))

	require.NoError(t, manager.CleanCache())
	assert.NoDirExists(t, stale)
	assert.DirExists(t, fresh)

	_, err = manager.GetResource("missing")
	require.Error(t, err)
}

func TestIdentifierFromDependencyMeta(t *testing.T) {
	t.Parallel()

	assert.Equal(t, filepath.Join("github.com", "u", "r"), identifierFromDependencyMeta(versioning.DependencyMeta{User: "u", Repo: "r"}))
	assert.Equal(t, filepath.Join("plugin", "github.com", "u", "r"), identifierFromDependencyMeta(versioning.DependencyMeta{Scheme: "plugin", User: "u", Repo: "r"}))
}

func createZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}
