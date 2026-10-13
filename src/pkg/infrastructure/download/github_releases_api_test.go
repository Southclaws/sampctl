package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type fakeReleasesAPI struct {
	releases []*github.RepositoryRelease
	byTag    map[string]*github.RepositoryRelease

	listCalls int
	tagCalls  int

	downloadCalls       int
	downloadRC          io.ReadCloser
	downloadRedirectURL string
	downloadErr         error
}

func (f *fakeReleasesAPI) ListReleases(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error) {
	f.listCalls++
	return f.releases, nil, nil
}

func (f *fakeReleasesAPI) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, *github.Response, error) {
	f.tagCalls++
	return f.byTag[tag], nil, nil
}

func (f *fakeReleasesAPI) DownloadReleaseAsset(ctx context.Context, owner, repo string, id int64) (io.ReadCloser, string, error) {
	f.downloadCalls++
	if f.downloadErr != nil {
		return nil, "", f.downloadErr
	}
	return f.downloadRC, f.downloadRedirectURL, nil
}

func TestReleaseAssetByPatternWithAPI_UsesListReleases_WhenNoTag(t *testing.T) {
	tmpDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	name := "asset-linux.zip"
	url := srv.URL + "/" + name
	rel := &github.RepositoryRelease{
		TagName: github.String("v1.2.3"),
		Assets: []github.ReleaseAsset{
			{ID: github.Int64(1), Name: github.String(name), BrowserDownloadURL: github.String(url)},
		},
	}
	fake := &fakeReleasesAPI{releases: []*github.RepositoryRelease{rel}, byTag: map[string]*github.RepositoryRelease{}}
	fake.downloadRC = io.NopCloser(strings.NewReader("ok"))

	filename, tag, err := ReleaseAssetByPatternWithAPI(ReleaseAssetAPIRequest{
		Context:    context.Background(),
		Client:     fake,
		Meta:       versioning.DependencyMeta{User: "o", Repo: "r"},
		Matcher:    regexp.MustCompile("linux"),
		Dir:        tmpDir,
		OutputFile: "",
		CacheDir:   tmpDir,
	})
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", tag)
	require.Equal(t, filepath.Join(tmpDir, name), filename)
	require.FileExists(t, filename)
	require.Equal(t, 1, fake.listCalls)
	require.Equal(t, 0, fake.tagCalls)
	require.Equal(t, 1, fake.downloadCalls)
}

func TestReleaseAssetByPatternWithAPI_UsesGetReleaseByTag_WhenTagProvided(t *testing.T) {
	tmpDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	name := "asset-win.zip"
	url := srv.URL + "/" + name
	rel := &github.RepositoryRelease{
		TagName: github.String("v9.9.9"),
		Assets: []github.ReleaseAsset{
			{ID: github.Int64(2), Name: github.String(name), BrowserDownloadURL: github.String(url)},
		},
	}
	fake := &fakeReleasesAPI{releases: nil, byTag: map[string]*github.RepositoryRelease{"v9.9.9": rel}}
	fake.downloadRC = io.NopCloser(strings.NewReader("ok"))

	filename, tag, err := ReleaseAssetByPatternWithAPI(ReleaseAssetAPIRequest{
		Context:    context.Background(),
		Client:     fake,
		Meta:       versioning.DependencyMeta{User: "o", Repo: "r", Tag: "v9.9.9"},
		Matcher:    regexp.MustCompile("win"),
		Dir:        tmpDir,
		OutputFile: "",
		CacheDir:   tmpDir,
	})
	require.NoError(t, err)
	require.Equal(t, "v9.9.9", tag)
	require.Equal(t, filepath.Join(tmpDir, name), filename)
	require.FileExists(t, filename)
	require.Equal(t, 0, fake.listCalls)
	require.Equal(t, 1, fake.tagCalls)
	require.Equal(t, 1, fake.downloadCalls)
}

func TestReleaseAssetByPatternWithAPI_ReturnsError_WhenNoAssetMatches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	name := "something.zip"
	url := srv.URL + "/" + name
	rel := &github.RepositoryRelease{
		TagName: github.String("v1.0.0"),
		Assets: []github.ReleaseAsset{
			{ID: github.Int64(3), Name: github.String(name), BrowserDownloadURL: github.String(url)},
		},
	}
	fake := &fakeReleasesAPI{releases: []*github.RepositoryRelease{rel}, byTag: map[string]*github.RepositoryRelease{}}
	fake.downloadRC = io.NopCloser(strings.NewReader("ok"))

	_, _, err := ReleaseAssetByPatternWithAPI(ReleaseAssetAPIRequest{
		Context:    context.Background(),
		Client:     fake,
		Meta:       versioning.DependencyMeta{User: "o", Repo: "r"},
		Matcher:    regexp.MustCompile("nope"),
		Dir:        "",
		OutputFile: "",
		CacheDir:   t.TempDir(),
	})
	require.Error(t, err)
}

func TestReleaseAssetByPatternWithAPIReportsMissingAssetInRequestedRelease(t *testing.T) {
	meta := versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-yaml", Tag: "1.0.0"}
	fake := &fakeReleasesAPI{
		byTag: map[string]*github.RepositoryRelease{
			"1.0.0": {
				TagName: github.String("1.0.0"),
				Assets:  []github.ReleaseAsset{{ID: github.Int64(1), Name: github.String("yaml.dll")}},
			},
		},
		releases: []*github.RepositoryRelease{{
			TagName: github.String("0.0.3"),
			Assets:  []github.ReleaseAsset{{ID: github.Int64(2), Name: github.String("yaml.so")}},
		}},
	}
	dir := t.TempDir()
	_, _, err := ReleaseAssetByPatternWithAPI(ReleaseAssetAPIRequest{
		Context: context.Background(),
		Client:  fake,
		Meta:    meta,
		Matcher: regexp.MustCompile(`yaml.so`),
		Dir:     dir,
	})
	var missing *ReleaseAssetNotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("ReleaseAssetByPatternWithAPI(%s, yaml.so) error = %v, want *ReleaseAssetNotFoundError", meta, err)
	}
	if missing.Meta != meta || missing.Tag != "1.0.0" || missing.Pattern != "yaml.so" {
		t.Errorf("ReleaseAssetByPatternWithAPI(%s, yaml.so) error = %+v, want requested metadata, tag 1.0.0, and pattern yaml.so", meta, missing)
	}
	if len(missing.AvailableAssets) != 1 || missing.AvailableAssets[0] != "yaml.dll" {
		t.Errorf("ReleaseAssetByPatternWithAPI(%s, yaml.so) available assets = %v, want [yaml.dll]", meta, missing.AvailableAssets)
	}
	if fake.tagCalls != 1 || fake.listCalls != 0 || fake.downloadCalls != 0 {
		t.Errorf("ReleaseAssetByPatternWithAPI(%s, yaml.so) calls = {tag:%d list:%d download:%d}, want {tag:1 list:0 download:0}",
			meta, fake.tagCalls, fake.listCalls, fake.downloadCalls)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir(%q) = %v, want no error", dir, readErr)
	}
	if len(entries) != 0 {
		t.Errorf("ReleaseAssetByPatternWithAPI(%s, yaml.so) output directory contains %v, want no files", meta, entries)
	}
}
