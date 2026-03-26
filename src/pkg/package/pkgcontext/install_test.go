package pkgcontext

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestPackageContextListRepositoryTagsFallsBackToAnonymousClient(t *testing.T) {
	t.Parallel()

	var authenticatedRequests int32
	var anonymousRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/fixture/dep/tags" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if r.Header.Get("Authorization") != "" {
			atomic.AddInt32(&authenticatedRequests, 1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}

		atomic.AddInt32(&anonymousRequests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"v1.2.3"}]`))
	}))
	defer server.Close()

	httpClient := oauth2.NewClient(
		context.Background(),
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "bad-token"}),
	)
	gh := github.NewClient(httpClient)
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = baseURL
	gh.UploadURL = baseURL

	pcx := PackageContext{PackageServices: PackageServices{GitHub: gh}}

	tags, err := pcx.listRepositoryTags(context.Background(), "fixture", "dep")
	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.NotNil(t, tags[0].Name)
	assert.Equal(t, "v1.2.3", *tags[0].Name)
	assert.EqualValues(t, 1, atomic.LoadInt32(&authenticatedRequests))
	assert.EqualValues(t, 1, atomic.LoadInt32(&anonymousRequests))
}
