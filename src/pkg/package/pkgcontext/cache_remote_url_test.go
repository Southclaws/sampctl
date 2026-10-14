package pkgcontext

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestCacheRefreshUsesDeclaredHTTPTransport(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		t.Run(map[bool]string{false: "anonymous", true: "configured HTTP credentials"}[authenticated], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cached")
			repo := cacheFixtureRepository(t, path, "")
			cfg, err := repo.Config()
			if err != nil {
				t.Fatal(err)
			}
			cfg.Remotes["origin"] = &gitconfig.RemoteConfig{Name: "origin", URLs: []string{"ssh://git@127.0.0.1:0/fixture/public"}}
			if err := repo.SetConfig(cfg); err != nil {
				t.Fatal(err)
			}

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				username, password, hasAuth := r.BasicAuth()
				if hasAuth != authenticated || authenticated && (username != "fixture" || password != "fixture-token") {
					t.Errorf("HTTP refresh authentication = (%q, %t), want configured=%t", username, hasAuth, authenticated)
				}

				http.NotFound(w, r)
			}))
			t.Cleanup(server.Close)

			pcx := PackageContext{}
			if authenticated {
				pcx.GitAuth = &gitHTTP.BasicAuth{Username: "fixture", Password: "fixture-token"}
			}

			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			_, err = pcx.ensureRepoExistsWithMeta(repoEnsureWithMetaRequest{
				Meta:              versioning.DependencyMeta{User: "fixture", Repo: "public"},
				repoEnsureRequest: repoEnsureRequest{Context: ctx, From: server.URL + "/fixture/public", To: path, ForceUpdate: true},
			})
			if got := requests.Load(); got != 1 || !errors.Is(err, transport.ErrRepositoryNotFound) {
				t.Errorf("ensureRepoExistsWithMeta(HTTP refresh) made %d HTTP requests with error %v, want one request and server's repository-not-found response", got, err)
			}
		})
	}
}

func TestCacheRefreshFetchesFromDeclaredSource(t *testing.T) {
	remotePath := filepath.Join(t.TempDir(), "remote")
	remote := cacheFixtureRepository(t, remotePath, "")
	cachePath := filepath.Join(t.TempDir(), "cached")
	cached, err := git.PlainClone(cachePath, false, &git.CloneOptions{URL: remotePath})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := cached.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Remotes["origin"].URLs = []string{filepath.Join(t.TempDir(), "missing-old-origin")}
	if err := cached.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(remotePath, "include.inc"), []byte("updated"), 0o600); err != nil {
		t.Fatal(err)
	}

	wt, err := remote.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := wt.Add("include.inc"); err != nil {
		t.Fatal(err)
	}

	if _, err := wt.Commit("advance source", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.invalid", When: time.Unix(200, 0)}}); err != nil {
		t.Fatal(err)
	}

	pcx := PackageContext{}
	if _, err := pcx.ensureRepoExists(repoEnsureRequest{Context: t.Context(), From: remotePath, To: cachePath, ForceUpdate: true}); err != nil {
		t.Fatalf("ensureRepoExists(declared source) error = %v, want nil", err)
	}

	if contents, err := os.ReadFile(filepath.Join(cachePath, "include.inc")); err != nil || string(contents) != "updated" {
		t.Errorf("ReadFile(refreshed cache) = %q, %v, want updated source content", contents, err)
	}
}
