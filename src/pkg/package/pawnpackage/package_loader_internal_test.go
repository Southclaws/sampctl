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

	pkg, err := GetRemotePackage(context.Background(), gh, versioning.DependencyMeta{User: "fixture", Repo: "repo"})
	require.NoError(t, err)
	require.Equal(t, "gamemodes/test.pwn", pkg.Entry)
	require.Equal(t, "gamemodes/test.amx", pkg.Output)
}

func TestGetRemotePackage_OfficialFallback_Offline(t *testing.T) {
	origBaseURL := officialPackageRepoBaseURL
	origClient := packageDefinitionHTTPClient
	defer func() {
		officialPackageRepoBaseURL = origBaseURL
		packageDefinitionHTTPClient = origClient
	}()

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
	officialPackageRepoBaseURL = server.URL + "/official"
	packageDefinitionHTTPClient = server.Client()

	pkg, err := GetRemotePackage(context.Background(), gh, versioning.DependencyMeta{User: "fixture", Repo: "repo"})
	require.NoError(t, err)
	require.Equal(t, fallbackPkg.Entry, pkg.Entry)
	require.Equal(t, fallbackPkg.Output, pkg.Output)
}
