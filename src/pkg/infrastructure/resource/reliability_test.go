package resource

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestHTTPFileResourceSeparatesURLPathsAndQueries(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	defer server.Close()

	cache := t.TempDir()
	identifiers := make(map[string]bool)
	for _, suffix := range []string{"/one/package.zip", "/two/package.zip", "/two/package.zip?variant=other"} {
		res, err := NewHTTPFileResource(server.URL+suffix, "v1", ResourceTypeArbitraryFile)
		if err != nil {
			t.Fatal(err)
		}
		res.SetCacheDir(cache)
		if identifiers[res.Identifier()] {
			t.Fatalf("distinct url %q reuses identifier %q", suffix, res.Identifier())
		}
		identifiers[res.Identifier()] = true

		if err := res.Ensure(t.Context(), "v1", ""); err != nil {
			t.Fatal(err)
		}
		_, path := res.Cached("v1")
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != suffix {
			t.Errorf("cached %s = %q, %v; want %q", suffix, contents, err, suffix)
		}
	}

	if got := requests.Load(); got != 3 {
		t.Errorf("http requests = %d; want 3 distinct downloads", got)
	}
}

type assetTestTransport func(*http.Request) (*http.Response, error)

func (transport assetTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type pausedAssetBody struct {
	blocked chan struct{}
	resume  chan struct{}
	reads   int
}

func (body *pausedAssetBody) Read(data []byte) (int, error) {
	body.reads++
	switch body.reads {
	case 1:
		return copy(data, "partial"), nil
	case 2:
		close(body.blocked)
		<-body.resume
		return copy(data, " complete"), nil
	default:
		return 0, io.EOF
	}
}

func (*pausedAssetBody) Close() error { return nil }

func TestGitHubReleaseCacheWaitsForCompletedDownload(t *testing.T) {
	t.Parallel()

	body := &pausedAssetBody{blocked: make(chan struct{}), resume: make(chan struct{})}
	var resume sync.Once
	t.Cleanup(func() { resume.Do(func() { close(body.resume) }) })
	client := github.NewClient(&http.Client{Transport: assetTestTransport(func(r *http.Request) (*http.Response, error) {
		response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r}
		if strings.Contains(r.URL.Path, "/assets/") {
			response.Header.Set("Content-Type", "application/octet-stream")
			response.Body = body
		} else {
			response.Header.Set("Content-Type", "application/json")
			response.Body = io.NopCloser(strings.NewReader(`{"tag_name":"v1","assets":[{"id":1,"name":"asset.zip"}]}`))
		}

		return response, nil
	})})
	cache := t.TempDir()
	newResource := func() *GitHubReleaseResource {
		res := NewGitHubReleaseResource(versioning.DependencyMeta{User: "u", Repo: "r", Tag: "v1"}, regexp.MustCompile(`asset`), ResourceTypePlugin, client)
		res.SetCacheDir(cache)
		return res
	}

	finished := make(chan error, 1)
	go func() { finished <- newResource().Ensure(t.Context(), "v1", "") }()
	select {
	case <-body.blocked:
	case err := <-finished:
		t.Fatalf("download ended before its body was read: %v", err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}

	if hit, path := newResource().Cached("v1"); hit {
		t.Errorf("unfinished transfer reported as cached: %s", path)
	}
	resume.Do(func() { close(body.resume) })
	if err := <-finished; err != nil {
		t.Fatal(err)
	}

	hit, path := newResource().Cached("v1")
	if !hit || filepath.Base(path) != "asset.zip" {
		t.Fatalf("completed asset cache = %v, %q; want asset.zip", hit, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "partial complete" {
		t.Errorf("completed asset = %q, %v", contents, err)
	}
}

func TestGitHubReleaseCacheIgnoresOrphanedAndUnrelatedFiles(t *testing.T) {
	t.Parallel()

	res := NewGitHubReleaseResource(versioning.DependencyMeta{User: "u", Repo: "r", Tag: "v1"}, regexp.MustCompile(`asset`), ResourceTypePlugin, nil)
	res.SetCacheDir(t.TempDir())
	cache := res.getCachePath("v1")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".asset.zip.123456", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(cache, name), []byte("incomplete or unrelated"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(cache, "asset-directory"), 0o700); err != nil {
		t.Fatal(err)
	}

	if hit, path := res.Cached("v1"); hit {
		t.Fatalf("uncommitted cache entries reported as complete: %s", path)
	}

	asset := filepath.Join(cache, "asset.zip")
	if err := os.WriteFile(asset, []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hit, path := res.Cached("v1"); !hit || path != asset {
		t.Errorf("completed asset cache = %v, %s; want %s", hit, path, asset)
	}
}

func TestGitHubReleaseCacheAcceptsCompletedHiddenAssets(t *testing.T) {
	t.Parallel()

	res := NewGitHubReleaseResource(versioning.DependencyMeta{User: "u", Repo: "r", Tag: "v1"}, regexp.MustCompile(`^\.asset\.zip$`), ResourceTypePlugin, nil)
	res.SetCacheDir(t.TempDir())
	cache := res.getCachePath("v1")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(cache, ".asset.zip")
	if err := os.WriteFile(asset, []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}

	if hit, path := res.Cached("v1"); !hit || path != asset {
		t.Errorf("completed hidden asset cache = %v, %q; want %q", hit, path, asset)
	}
}

func TestFactoryDownloadsThePinnedGitHubAsset(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/user/repo/releases/tags/v1.0.0":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"tag_name":"v1.0.0","assets":[{"id":1,"name":"other.zip"},{"id":2,"name":"tool.zip"}]}`)
		case "/repos/user/repo/releases/assets/2":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "pinned tool")
		default:
			t.Errorf("unexpected github request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := github.NewClient(server.Client())
	base, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = base

	factory := NewDefaultResourceFactory(client)
	res, err := factory.FromURL("https://github.com/user/repo/releases/download/v1.0.0/tool.zip", ResourceTypeCompiler)
	if err != nil {
		t.Fatal(err)
	}
	res.(*GitHubReleaseResource).SetCacheDir(t.TempDir())
	target := filepath.Join(t.TempDir(), "tool.zip")
	if err := res.Ensure(context.Background(), res.Version(), target); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "pinned tool" {
		t.Errorf("requested asset = %q, %v; want pinned tool", contents, err)
	}
}

func TestFactoryPreservesEscapedReleaseNamesAndLatestAsset(t *testing.T) {
	t.Parallel()

	factory := NewDefaultResourceFactory(nil)
	for _, test := range []struct {
		url, version, filename string
	}{
		{"https://github.com/user/repo/releases/download/feature%2Fv1/tool%2Bdebug.zip", "feature/v1", "tool+debug.zip"},
		{"https://github.com/user/repo/releases/latest/download/tool.zip", "latest", "tool.zip"},
	} {
		res, err := factory.FromURL(test.url, ResourceTypePlugin)
		if err != nil {
			t.Fatal(err)
		}
		release := res.(*GitHubReleaseResource)
		if release.Version() != test.version || !release.assetPattern.MatchString(test.filename) || release.assetPattern.MatchString("prefix"+test.filename) {
			t.Errorf("resource for %s has version %s and matcher %s", test.url, release.Version(), release.assetPattern)
		}
	}

	res, err := factory.FromURL("https://github.com.example.invalid/user/repo/releases/download/v1/tool.zip", ResourceTypePlugin)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.(*HTTPResource); !ok {
		t.Errorf("unrelated host produced %T; want http resource", res)
	}
}
