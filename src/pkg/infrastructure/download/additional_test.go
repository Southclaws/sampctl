package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestExtractFuncFromName(t *testing.T) {
	assert.NotNil(t, ExtractFuncFromName(ExtractZip))
	assert.NotNil(t, ExtractFuncFromName(ExtractTgz))
	assert.Nil(t, ExtractFuncFromName("unknown"))
}

func TestFromCache(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "plugin.zip")
	createZipArchive(t, archivePath, map[string]string{"pkg/bin/tool": "payload"})

	cacheDir := filepath.Dir(archivePath)
	hit, err := FromCache(cacheDir, filepath.Base(archivePath), t.TempDir(), Unzip, map[string]string{"pkg/bin/tool": "bin/"}, "linux")
	require.NoError(t, err)
	assert.True(t, hit)

	hit, err = FromCache(cacheDir, "missing.zip", t.TempDir(), Unzip, nil, "linux")
	require.NoError(t, err)
	assert.False(t, hit)
}

func TestArchiveWrapperHelpers(t *testing.T) {
	tarPath := filepath.Join(t.TempDir(), "archive.tar.gz")
	createTarArchive(t, tarPath, true, map[string]string{"pkg/file.txt": "tar"})
	files, err := Untar(tarPath, t.TempDir(), map[string]string{"pkg/file.txt": "out.txt"})
	require.NoError(t, err)
	assert.Len(t, files, 1)

	zipPath := filepath.Join(t.TempDir(), "archive.zip")
	createZipArchive(t, zipPath, map[string]string{"pkg/file.txt": "zip"})
	files, err = Unzip(zipPath, t.TempDir(), map[string]string{"pkg/file.txt": "out.txt"})
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

func TestPackageCacheHelpers(t *testing.T) {
	cacheDir := t.TempDir()
	body := `[{"repo":"repo","user":"user"}]`
	client := roundTripClient(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "list.packages.sampctl.com", req.URL.Host)
		return jsonResponse(http.StatusOK, body), nil
	})

	pkgs, err := GetPackageListWithClient(cacheDir, client)
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "repo", pkgs[0].Repo)

	require.NoError(t, UpdatePackageListWithClient(cacheDir, client))
	require.NoError(t, WritePackageCacheFile(cacheDir, []byte(body)))
	assert.FileExists(t, filepath.Join(cacheDir, "packages.json"))

	pkgs, err = GetPackageList(cacheDir)
	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	_, err = GetPackageListWithClient(t.TempDir(), roundTripClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadGateway, `{}`), nil
	}))
	require.Error(t, err)
}

func TestUpdatePackageList(t *testing.T) {
	origClient := http.DefaultClient
	defer func() { http.DefaultClient = origClient }()

	http.DefaultClient = &http.Client{Transport: roundTripClient(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "list.packages.sampctl.com", req.URL.Host)
		return jsonResponse(http.StatusOK, `[{"repo":"repo2","user":"user2"}]`), nil
	})}

	cacheDir := t.TempDir()
	require.NoError(t, UpdatePackageList(cacheDir))
	pkgs, err := GetPackageList(cacheDir)
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "repo2", pkgs[0].Repo)
}

func TestFromNetWithClientRetryBranches(t *testing.T) {
	t.Run("invalid request url", func(t *testing.T) {
		_, err := FromNetWithClient(context.Background(), roundTripClient(func(req *http.Request) (*http.Response, error) {
			return nil, nil
		}), "://bad", filepath.Join(t.TempDir(), "out.bin"))
		require.Error(t, err)
	})

	t.Run("retries on unexpected content type", func(t *testing.T) {
		var hits int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			if hits < 3 {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write([]byte("png"))
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		oldMax := fromNetMaxAttempts
		oldSleep := fromNetSleep
		oldBackoff := fromNetBackoff
		defer func() {
			fromNetMaxAttempts = oldMax
			fromNetSleep = oldSleep
			fromNetBackoff = oldBackoff
		}()

		fromNetMaxAttempts = 3
		fromNetSleep = func(time.Duration) {}
		fromNetBackoff = func(int) time.Duration { return 0 }

		out := filepath.Join(t.TempDir(), "out.bin")
		got, err := FromNetWithClient(context.Background(), server.Client(), server.URL, out)
		require.NoError(t, err)
		assert.Equal(t, out, got)
		assert.Equal(t, 3, hits)
	})

	t.Run("uses retry after header", func(t *testing.T) {
		var hits int
		var sleeps []time.Duration
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			if hits == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("slow down"))
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		oldMax := fromNetMaxAttempts
		oldSleep := fromNetSleep
		oldBackoff := fromNetBackoff
		defer func() {
			fromNetMaxAttempts = oldMax
			fromNetSleep = oldSleep
			fromNetBackoff = oldBackoff
		}()

		fromNetMaxAttempts = 2
		fromNetSleep = func(d time.Duration) { sleeps = append(sleeps, d) }
		fromNetBackoff = func(int) time.Duration { return 0 }

		_, err := FromNetWithClient(context.Background(), server.Client(), server.URL, filepath.Join(t.TempDir(), "out.bin"))
		require.NoError(t, err)
		require.Len(t, sleeps, 1)
		assert.Equal(t, time.Second, sleeps[0])
		assert.Equal(t, 2, hits)
	})
}

func TestCompilerAndRuntimeCacheHelpers(t *testing.T) {
	origClient := http.DefaultClient
	defer func() { http.DefaultClient = origClient }()

	compilerBody := `{"linux":{"match":"compiler.zip","method":"zip","binary":"pawncc"}}`
	runtimeBody := `{"aliases":{"latest":"0.3.7"},"packages":[{"version":"0.3.7","linux":"server.tar.gz"}]}`

	http.DefaultClient = &http.Client{Transport: roundTripClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/sampctl/compilers/master/compilers.json":
			return jsonResponse(http.StatusOK, compilerBody), nil
		case "/sampctl/runtimes/master/runtimes.json":
			return jsonResponse(http.StatusOK, runtimeBody), nil
		default:
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
	})}

	compilerDir := t.TempDir()
	compilers, err := GetCompilerList(compilerDir)
	require.NoError(t, err)
	assert.Equal(t, "compiler.zip", compilers["linux"].Match)
	require.NoError(t, UpdateCompilerList(compilerDir))
	require.NoError(t, WriteCompilerCacheFile(compilerDir, []byte(compilerBody)))

	runtimeDir := t.TempDir()
	runtimes, err := GetRuntimeList(runtimeDir)
	require.NoError(t, err)
	assert.Equal(t, "0.3.7", runtimes.Aliases["latest"])
	require.NoError(t, UpdateRuntimeList(runtimeDir))
	require.NoError(t, WriteRuntimeCacheFile(runtimeDir, []byte(runtimeBody)))
}

func TestGitHubClientReleasesAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases":
			_, _ = io.WriteString(w, `[{"tag_name":"v1.0.0"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/tags/v1.0.0":
			_, _ = io.WriteString(w, `{"tag_name":"v1.0.0"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/assets/12":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "asset")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := github.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")
	client.UploadURL = mustParseURL(t, server.URL+"/")
	api := githubClientReleasesAdapter{client: client}

	releases, _, err := api.ListReleases(context.Background(), "o", "r", &github.ListOptions{})
	require.NoError(t, err)
	require.Len(t, releases, 1)

	release, _, err := api.GetReleaseByTag(context.Background(), "o", "r", "v1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", release.GetTagName())

	rc, _, err := api.DownloadReleaseAsset(context.Background(), "o", "r", 12)
	require.NoError(t, err)
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	_ = rc.Close()
	assert.Equal(t, []byte("asset"), data)
}

func TestReleaseAssetByPattern(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases":
			_, _ = io.WriteString(w, `[{"tag_name":"v1.0.0","assets":[{"id":1,"name":"tool.zip","browser_download_url":"http://example.invalid/tool.zip"}]}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases/assets/1":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "asset-body")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := github.NewClient(server.Client())
	client.BaseURL = mustParseURL(t, server.URL+"/")
	client.UploadURL = mustParseURL(t, server.URL+"/")

	dir := t.TempDir()
	file, tag, err := ReleaseAssetByPattern(context.Background(), client, versioning.DependencyMeta{User: "o", Repo: "r"}, regexp.MustCompile(`tool`), dir, "", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", tag)
	assert.Equal(t, filepath.Join(dir, "tool.zip"), file)
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, []byte("asset-body"), data)
}

func TestReleaseAssetAndReleaseLookupErrors(t *testing.T) {
	t.Run("download release asset branches", func(t *testing.T) {
		meta := versioning.DependencyMeta{User: "u", Repo: "r"}

		_, err := downloadReleaseAsset(context.Background(), nil, meta, nil, filepath.Join(t.TempDir(), "out.bin"))
		require.Error(t, err)

		api := &fakeReleasesAPI{downloadErr: errors.New("boom")}
		asset := &github.ReleaseAsset{ID: github.Int64(1), Name: github.String("asset.zip")}
		_, err = downloadReleaseAsset(context.Background(), api, meta, asset, filepath.Join(t.TempDir(), "out.bin"))
		require.Error(t, err)

		api = &fakeReleasesAPI{}
		_, err = downloadReleaseAsset(context.Background(), api, meta, asset, filepath.Join(t.TempDir(), "out.bin"))
		require.ErrorContains(t, err, "empty response body")

		asset = &github.ReleaseAsset{Name: github.String("asset.zip")}
		_, err = downloadReleaseAsset(context.Background(), nil, meta, asset, filepath.Join(t.TempDir(), "out.bin"))
		require.ErrorContains(t, err, "no download URL")
	})

	t.Run("redirect download path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("redirected"))
		}))
		defer server.Close()

		oldFactory := fromNetClientFactory
		defer func() { fromNetClientFactory = oldFactory }()
		fromNetClientFactory = func() *http.Client { return server.Client() }

		api := &fakeReleasesAPI{downloadRedirectURL: server.URL}
		asset := &github.ReleaseAsset{ID: github.Int64(1), Name: github.String("asset.zip")}
		out := filepath.Join(t.TempDir(), "out.bin")
		got, err := downloadReleaseAsset(context.Background(), api, versioning.DependencyMeta{User: "u", Repo: "r"}, asset, out)
		require.NoError(t, err)
		assert.Equal(t, out, got)
		data, readErr := os.ReadFile(out)
		require.NoError(t, readErr)
		assert.Equal(t, "redirected", string(data))
	})

	t.Run("latest release lookup errors", func(t *testing.T) {
		_, err := getLatestReleaseOrPreRelease(context.Background(), errorReleasesAPI{}, "u", "r")
		require.Error(t, err)

		_, err = getLatestReleaseOrPreRelease(context.Background(), &fakeReleasesAPI{}, "u", "r")
		require.ErrorContains(t, err, "no releases available")
	})

	t.Run("release asset wrapper uses basename output file", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("custom-name"))
		}))
		defer server.Close()

		oldFactory := fromNetClientFactory
		defer func() { fromNetClientFactory = oldFactory }()
		fromNetClientFactory = func() *http.Client { return server.Client() }

		assetURL := server.URL + "/asset.zip"
		fake := &fakeReleasesAPI{releases: []*github.RepositoryRelease{{
			TagName: github.String("v1.0.0"),
			Assets:  []github.ReleaseAsset{{Name: github.String("asset.zip"), BrowserDownloadURL: &assetURL}},
		}}}

		file, _, err := ReleaseAssetByPatternWithAPI(context.Background(), fake, versioning.DependencyMeta{User: "u", Repo: "r"}, regexp.MustCompile(`asset`), "", "nested/custom.bin", t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, "custom.bin", filepath.Base(file))
	})
}

type errorReleasesAPI struct{}

func (errorReleasesAPI) ListReleases(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error) {
	return nil, nil, errors.New("list failed")
}

func (errorReleasesAPI) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, *github.Response, error) {
	return nil, nil, errors.New("tag failed")
}

func (errorReleasesAPI) DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, string, error) {
	return nil, "", errors.New("download failed")
}

type roundTripClient func(*http.Request) (*http.Response, error)

func (fn roundTripClient) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func (fn roundTripClient) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
