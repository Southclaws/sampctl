package pawnpackage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestRemotePackageFetchPreservesRateLimit(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	reset := time.Now().Add(time.Hour).Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": "API rate limit exceeded for fixture"}); err != nil {
			t.Errorf("rate limit response encode error = %v", err)
		}
	}))
	t.Cleanup(server.Close)
	fetcher := testRemotePackageFetcher(t, server)
	_, err := fetcher.Fetch(context.Background(), versioning.DependencyMeta{User: "fixture", Repo: "plugin"})
	var rateLimit *github.RateLimitError
	if !errors.As(err, &rateLimit) {
		t.Fatalf("Fetch(rate-limited repository) error = %v, want *github.RateLimitError", err)
	}
	if !rateLimit.Rate.Reset.Time.Equal(reset) || rateLimit.Rate.Limit != 60 {
		t.Errorf("Fetch(rate-limited repository) rate = %v, want limit 60 and reset %s", rateLimit.Rate, reset)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("Fetch(rate-limited repository) requests = %d, want one request with no fallback", got)
	}
}

func TestRemotePackageFetchPreservesInvalidDefinition(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		content := base64.StdEncoding.EncodeToString([]byte(`{"entry":`))
		if err := json.NewEncoder(w).Encode(map[string]string{"type": "file", "encoding": "base64", "content": content}); err != nil {
			t.Errorf("invalid definition response encode error = %v", err)
		}
	}))
	t.Cleanup(server.Close)
	fetcher := testRemotePackageFetcher(t, server)
	_, err := fetcher.Fetch(context.Background(), versioning.DependencyMeta{User: "fixture", Repo: "plugin"})
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		t.Errorf("Fetch(invalid repository definition) error = %v, want *json.SyntaxError", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("Fetch(invalid repository definition) requests = %d, want one request with no fallback", got)
	}
}

func TestRemotePackageFetchPreservesCancellation(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	fetcher := testRemotePackageFetcher(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetcher.Fetch(ctx, versioning.DependencyMeta{User: "fixture", Repo: "plugin"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch(canceled context) error = %v, want context.Canceled", err)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("Fetch(canceled context) requests = %d, want none", got)
	}
}

func testRemotePackageFetcher(t *testing.T, server *httptest.Server) *GitHubRemotePackageFetcher {
	t.Helper()
	client := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = baseURL
	return &GitHubRemotePackageFetcher{GitHub: client, HTTPClient: server.Client(), OfficialBaseURL: server.URL + "/official"}
}
