package rook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/file"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitSSH "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestGetCanceledBeforeCloneDoesNotCreateDestination(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	target := filepath.Join(t.TempDir(), "destination")

	err := Get(GetOptions{Context: ctx, Meta: versioning.DependencyMeta{Site: "example.invalid", User: "u", Repo: "r"}, Dir: target})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("get with canceled context = %v; want cancellation", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("canceled get created destination: %v", err)
	}
}

func TestClonePackageRepositorySendsHTTPAuthentication(t *testing.T) {
	var authenticated atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "fixture" || password != "fixture-token" {
			w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		authenticated.Store(true)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	installCloneTestProtocol(t, gitHTTP.NewClient(server.Client()))

	_, err := clonePackageRepository(GetOptions{
		Context: t.Context(),
		Meta:    versioning.DependencyMeta{Site: strings.TrimPrefix(server.URL, "https://"), User: "u", Repo: "private"},
		Dir:     filepath.Join(t.TempDir(), "clone"),
		Auth:    &gitHTTP.BasicAuth{Username: "fixture", Password: "fixture-token"},
	})
	if !authenticated.Load() {
		t.Errorf("clone did not send configured credentials; error = %v", err)
	}
	if !errors.Is(err, transport.ErrRepositoryNotFound) {
		t.Errorf("authenticated fixture response = %v; want repository not found", err)
	}
}

func TestClonePackageRepositoryCancelsTransfer(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	installCloneTestProtocol(t, gitHTTP.NewClient(server.Client()))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := clonePackageRepository(GetOptions{
			Context: ctx,
			Meta:    versioning.DependencyMeta{Site: strings.TrimPrefix(server.URL, "https://"), User: "u", Repo: "r"},
			Dir:     filepath.Join(t.TempDir(), "clone"),
		})
		finished <- err
	}()
	<-started
	cancel()

	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Errorf("clone cancellation = %v; want context canceled", err)
	}
	<-stopped
}

func TestPackageCloneAuthUsesRemoteProtocol(t *testing.T) {
	httpAuth := &gitHTTP.BasicAuth{Username: "u", Password: "token"}
	sshAuth := &gitSSH.Password{User: "git", Password: "ssh-token"}
	bundle := &pkgcontext.GitMultiAuth{HTTP: httpAuth, SSH: sshAuth}
	for _, test := range []struct {
		name string
		ssh  string
		auth transport.AuthMethod
		want transport.AuthMethod
	}{
		{name: "http", auth: httpAuth, want: httpAuth},
		{name: "ssh", ssh: "git", auth: sshAuth, want: sshAuth},
		{name: "http bundle", auth: bundle, want: httpAuth},
		{name: "ssh bundle", ssh: "git", auth: bundle, want: sshAuth},
		{name: "http credentials on ssh", ssh: "git", auth: httpAuth},
		{name: "ssh credentials on http", auth: sshAuth},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := packageCloneAuth(versioning.DependencyMeta{SSH: test.ssh}, test.auth); got != test.want {
				t.Errorf("selected auth = %T; want %T", got, test.want)
			}
		})
	}
}

type cloneFixtureTransport struct {
	transport.Transport
	path string
}

func (fixture cloneFixtureTransport) NewUploadPackSession(endpoint *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	redirected := *endpoint
	redirected.Path = fixture.path

	return fixture.Transport.NewUploadPackSession(&redirected, auth)
}

func installCloneTestProtocol(t *testing.T, protocol transport.Transport) {
	t.Helper()

	original := client.Protocols["https"]
	client.InstallProtocol("https", protocol)
	t.Cleanup(func() { client.InstallProtocol("https", original) })
}

func TestClonePackageRepositoryChecksOutRequestedRevision(t *testing.T) {
	remote := t.TempDir()
	repo, err := git.PlainInit(remote, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	commit := func(contents string) plumbing.Hash {
		t.Helper()

		if err := os.WriteFile(filepath.Join(remote, "marker.pwn"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := worktree.Add("marker.pwn"); err != nil {
			t.Fatal(err)
		}
		hash, err := worktree.Commit(contents, &git.CommitOptions{Author: &object.Signature{Name: "fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}})
		if err != nil {
			t.Fatal(err)
		}

		return hash
	}
	first := commit("first release")
	if _, err := repo.CreateTag("v1.0.0", first, &git.CreateTagOptions{Tagger: &object.Signature{Name: "fixture", Email: "fixture@example.invalid", When: time.Unix(1, 0)}, Message: "annotated release"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("old"), first)); err != nil {
		t.Fatal(err)
	}
	second := commit("newest compatible release")
	if _, err := repo.CreateTag("v1.2.0", second, nil); err != nil {
		t.Fatal(err)
	}
	latest := commit("default branch")
	installCloneTestProtocol(t, cloneFixtureTransport{Transport: file.DefaultClient, path: remote})

	for _, test := range []struct {
		name    string
		meta    versioning.DependencyMeta
		want    plumbing.Hash
		wantErr bool
	}{
		{name: "default", want: latest},
		{name: "annotated tag", meta: versioning.DependencyMeta{Tag: "v1.0.0"}, want: first},
		{name: "semantic constraint", meta: versioning.DependencyMeta{Tag: "^1.0.0"}, want: second},
		{name: "latest release", meta: versioning.DependencyMeta{Tag: "latest"}, want: second},
		{name: "branch", meta: versioning.DependencyMeta{Branch: "old"}, want: first},
		{name: "commit", meta: versioning.DependencyMeta{Commit: first.String()}, want: first},
		{name: "missing tag", meta: versioning.DependencyMeta{Tag: "missing"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			meta := test.meta
			meta.Site, meta.User, meta.Repo = "fixture.invalid", "u", "r"
			cloned, err := clonePackageRepository(GetOptions{Context: t.Context(), Meta: meta, Dir: filepath.Join(t.TempDir(), "clone")})
			if test.wantErr {
				if err == nil {
					t.Fatal("missing requested revision unexpectedly succeeded")
				}

				return
			}
			if err != nil {
				t.Fatal(err)
			}
			head, err := cloned.Head()
			if err != nil || head.Hash() != test.want {
				t.Errorf("checked out head = %v, %v; want %s", head, err, test.want)
			}
		})
	}

	for _, name := range []string{"v1.0.0", "v1.2.0"} {
		if err := repo.DeleteTag(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("latest without tags uses default branch", func(t *testing.T) {
		cloned, err := clonePackageRepository(GetOptions{Context: t.Context(), Meta: versioning.DependencyMeta{Site: "fixture.invalid", User: "u", Repo: "r", Tag: "latest"}, Dir: filepath.Join(t.TempDir(), "clone")})
		if err != nil {
			t.Fatal(err)
		}
		head, err := cloned.Head()
		if err != nil || head.Hash() != latest {
			t.Errorf("latest without tags = %v, %v; want default branch %s", head, err, latest)
		}
	})

	for name, hash := range map[string]plumbing.Hash{"alpha": first, "beta": second} {
		if _, err := repo.CreateTag(name, hash, nil); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("latest nonsemantic tag uses deterministic ordering", func(t *testing.T) {
		cloned, err := clonePackageRepository(GetOptions{Context: t.Context(), Meta: versioning.DependencyMeta{Site: "fixture.invalid", User: "u", Repo: "r", Tag: "latest"}, Dir: filepath.Join(t.TempDir(), "clone")})
		if err != nil {
			t.Fatal(err)
		}
		head, err := cloned.Head()
		if err != nil || head.Hash() != second {
			t.Errorf("latest nonsemantic tag = %v, %v; want beta %s", head, err, second)
		}
	})
}
