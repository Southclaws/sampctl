package pawnpackage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestGetRemotePackage_FromRepo_Offline(t *testing.T) {
	pkgJSON := `{"entry":"gamemodes/test.pwn","output":"gamemodes/test.amx"}`
	content := base64.StdEncoding.EncodeToString([]byte(pkgJSON))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/fixture/repo/contents/pawn.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + content + `"}`))
	}))
	defer server.Close()

	gh := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = baseURL

	fetcher := &GitHubRemotePackageFetcher{GitHub: gh, HTTPClient: server.Client(), OfficialBaseURL: server.URL + "/official"}
	pkg, err := fetcher.Fetch(context.Background(), versioning.DependencyMeta{User: "fixture", Repo: "repo"})
	require.NoError(t, err)
	require.Equal(t, "gamemodes/test.pwn", pkg.Entry)
	require.Equal(t, "gamemodes/test.amx", pkg.Output)
}

func TestGetRemotePackage_FromRepo_PrefersCommitThenBranchThenTag(t *testing.T) {
	t.Parallel()

	commitPkgJSON := `{"entry":"gamemodes/commit.pwn","output":"gamemodes/commit.amx"}`
	branchPkgJSON := `{"entry":"gamemodes/branch.pwn","output":"gamemodes/branch.amx"}`
	tagPkgJSON := `{"entry":"gamemodes/tag.pwn","output":"gamemodes/tag.amx"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/fixture/repo/contents/pawn.json" {
			http.NotFound(w, r)
			return
		}

		var pkgJSON string
		switch r.URL.Query().Get("ref") {
		case "deadbeef":
			pkgJSON = commitPkgJSON
		case "release":
			pkgJSON = branchPkgJSON
		case "v1.2.3":
			pkgJSON = tagPkgJSON
		default:
			http.NotFound(w, r)
			return
		}

		content := base64.StdEncoding.EncodeToString([]byte(pkgJSON))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"file","encoding":"base64","content":"` + content + `"}`))
	}))
	defer server.Close()

	gh := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = baseURL

	fetcher := &GitHubRemotePackageFetcher{GitHub: gh, HTTPClient: server.Client(), OfficialBaseURL: server.URL + "/official"}
	pkg, err := fetcher.Fetch(context.Background(), versioning.DependencyMeta{
		User:   "fixture",
		Repo:   "repo",
		Tag:    "v1.2.3",
		Branch: "release",
		Commit: "deadbeef",
	})
	require.NoError(t, err)
	require.Equal(t, "gamemodes/commit.pwn", pkg.Entry)
	require.Equal(t, "gamemodes/commit.amx", pkg.Output)

	pkg, err = fetcher.Fetch(context.Background(), versioning.DependencyMeta{
		User:   "fixture",
		Repo:   "repo",
		Tag:    "v1.2.3",
		Branch: "release",
	})
	require.NoError(t, err)
	require.Equal(t, "gamemodes/branch.pwn", pkg.Entry)
	require.Equal(t, "gamemodes/branch.amx", pkg.Output)
}

func TestRemoteDefinitionRefsOrder(t *testing.T) {
	t.Parallel()

	refs := remoteDefinitionRefs(versioning.DependencyMeta{
		Tag:    "v1.2.3",
		Branch: "release",
		Commit: "deadbeef",
	})
	require.Equal(t, []string{"deadbeef", "release", "v1.2.3", ""}, refs)

	refs = remoteDefinitionRefs(versioning.DependencyMeta{Branch: "release", Tag: "v1.2.3"})
	require.Equal(t, []string{"release", "v1.2.3", ""}, refs)
}

func TestGetRemotePackage_OfficialFallback_Offline(t *testing.T) {
	fallbackPkg := Package{Entry: "filterscripts/test.pwn", Output: "filterscripts/test.amx"}
	fallbackJSON, err := json.Marshal(fallbackPkg)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/fixture/repo/contents/pawn.json", "/repos/fixture/repo/contents/pawn.yaml":
			http.NotFound(w, r)
		case "/official/fixture-repo.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(fallbackJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	gh := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = baseURL
	fetcher := &GitHubRemotePackageFetcher{GitHub: gh, HTTPClient: server.Client(), OfficialBaseURL: server.URL + "/official"}

	pkg, err := fetcher.Fetch(context.Background(), versioning.DependencyMeta{User: "fixture", Repo: "repo"})
	require.NoError(t, err)
	require.Equal(t, fallbackPkg.Entry, pkg.Entry)
	require.Equal(t, fallbackPkg.Output, pkg.Output)
}
