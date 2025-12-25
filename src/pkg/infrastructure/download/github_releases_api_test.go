package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
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
}

func (f *fakeReleasesAPI) ListReleases(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.RepositoryRelease, *github.Response, error) {
	f.listCalls++
	return f.releases, nil, nil
}

func (f *fakeReleasesAPI) GetReleaseByTag(ctx context.Context, owner, repo, tag string) (*github.RepositoryRelease, *github.Response, error) {
	f.tagCalls++
	return f.byTag[tag], nil, nil
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
			{Name: github.String(name), BrowserDownloadURL: github.String(url)},
		},
	}
	fake := &fakeReleasesAPI{releases: []*github.RepositoryRelease{rel}, byTag: map[string]*github.RepositoryRelease{}}

	filename, tag, err := ReleaseAssetByPatternWithAPI(
		context.Background(),
		fake,
		versioning.DependencyMeta{User: "o", Repo: "r"},
		regexp.MustCompile("linux"),
		tmpDir,
		"",
		tmpDir,
	)
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", tag)
	require.Equal(t, filepath.Join(tmpDir, name), filename)
	require.FileExists(t, filename)
	require.Equal(t, 1, fake.listCalls)
	require.Equal(t, 0, fake.tagCalls)
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
			{Name: github.String(name), BrowserDownloadURL: github.String(url)},
		},
	}
	fake := &fakeReleasesAPI{releases: nil, byTag: map[string]*github.RepositoryRelease{"v9.9.9": rel}}

	filename, tag, err := ReleaseAssetByPatternWithAPI(
		context.Background(),
		fake,
		versioning.DependencyMeta{User: "o", Repo: "r", Tag: "v9.9.9"},
		regexp.MustCompile("win"),
		tmpDir,
		"",
		tmpDir,
	)
	require.NoError(t, err)
	require.Equal(t, "v9.9.9", tag)
	require.Equal(t, filepath.Join(tmpDir, name), filename)
	require.FileExists(t, filename)
	require.Equal(t, 0, fake.listCalls)
	require.Equal(t, 1, fake.tagCalls)
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
			{Name: github.String(name), BrowserDownloadURL: github.String(url)},
		},
	}
	fake := &fakeReleasesAPI{releases: []*github.RepositoryRelease{rel}, byTag: map[string]*github.RepositoryRelease{}}

	_, _, err := ReleaseAssetByPatternWithAPI(
		context.Background(),
		fake,
		versioning.DependencyMeta{User: "o", Repo: "r"},
		regexp.MustCompile("nope"),
		"",
		"",
		t.TempDir(),
	)
	require.Error(t, err)
}
