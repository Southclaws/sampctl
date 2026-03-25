package pkgcontext

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestPackageContext_shouldRetryWithSSH(t *testing.T) {
	t.Parallel()

	meta := versioning.DependencyMeta{Site: "github.com", User: "owner", Repo: "private-repo"}
	authErr := errors.New("authentication required: Repository not found")

	t.Run("retries when HTTPS auth fails and SSH auth exists", func(t *testing.T) {
		t.Parallel()
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: &ssh.PublicKeys{User: "git"}}}
		shouldRetry := pcx.shouldRetryWithSSH(meta, "https://github.com/owner/private-repo", false, authErr)
		assert.True(t, shouldRetry)
	})

	t.Run("does not retry when already using SSH", func(t *testing.T) {
		t.Parallel()
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: &ssh.PublicKeys{User: "git"}}}
		shouldRetry := pcx.shouldRetryWithSSH(meta, "https://github.com/owner/private-repo", true, authErr)
		assert.False(t, shouldRetry)
	})

	t.Run("does not retry for non-auth error", func(t *testing.T) {
		t.Parallel()
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: &ssh.PublicKeys{User: "git"}}}
		shouldRetry := pcx.shouldRetryWithSSH(meta, "https://github.com/owner/private-repo", false, errors.New("network timeout"))
		assert.False(t, shouldRetry)
	})

	t.Run("does not retry without SSH auth method", func(t *testing.T) {
		t.Parallel()
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: &http.BasicAuth{Username: "u", Password: "p"}}}
		shouldRetry := pcx.shouldRetryWithSSH(meta, "https://github.com/owner/private-repo", false, authErr)
		assert.False(t, shouldRetry)
	})

	t.Run("retries when auth bundle has SSH auth", func(t *testing.T) {
		t.Parallel()
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: &GitMultiAuth{
			HTTP: &http.BasicAuth{Username: "u", Password: "p"},
			SSH:  &ssh.PublicKeys{User: "git"},
		}}}
		shouldRetry := pcx.shouldRetryWithSSH(meta, "https://github.com/owner/private-repo", false, authErr)
		assert.True(t, shouldRetry)
	})
}

func TestToGitSSHURL(t *testing.T) {
	t.Parallel()

	t.Run("uses default git user and github host", func(t *testing.T) {
		t.Parallel()
		got := toGitSSHURL(versioning.DependencyMeta{User: "owner", Repo: "repo"})
		assert.Equal(t, "git@github.com:owner/repo", got)
	})

	t.Run("uses explicit SSH user and custom host", func(t *testing.T) {
		t.Parallel()
		got := toGitSSHURL(versioning.DependencyMeta{Site: "gitlab.com", SSH: "deploy", User: "owner", Repo: "repo"})
		assert.Equal(t, "deploy@gitlab.com:owner/repo", got)
	})
}

func TestPackageContext_authForRemote(t *testing.T) {
	t.Parallel()

	t.Run("uses HTTP auth for HTTPS remotes", func(t *testing.T) {
		t.Parallel()
		httpAuth := &http.BasicAuth{Username: "u", Password: "p"}
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: httpAuth}}
		got := pcx.authForRemote("https://github.com/owner/private-repo", false)
		assert.Equal(t, httpAuth, got)
	})

	t.Run("does not use HTTP auth for SSH remotes", func(t *testing.T) {
		t.Parallel()
		httpAuth := &http.BasicAuth{Username: "u", Password: "p"}
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: httpAuth}}
		got := pcx.authForRemote("git@github.com:owner/private-repo", true)
		assert.Nil(t, got)
	})

	t.Run("uses SSH auth for SSH remotes", func(t *testing.T) {
		t.Parallel()
		sshAuth := &ssh.PublicKeys{User: "git"}
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: sshAuth}}
		got := pcx.authForRemote("git@github.com:owner/private-repo", true)
		assert.Equal(t, sshAuth, got)
	})

	t.Run("does not use SSH auth for HTTPS remotes", func(t *testing.T) {
		t.Parallel()
		sshAuth := &ssh.PublicKeys{User: "git"}
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: sshAuth}}
		got := pcx.authForRemote("https://github.com/owner/private-repo", false)
		assert.Nil(t, got)
	})

	t.Run("uses both auth methods from bundle", func(t *testing.T) {
		t.Parallel()
		httpAuth := &http.BasicAuth{Username: "u", Password: "p"}
		sshAuth := &ssh.PublicKeys{User: "git"}
		pcx := PackageContext{PackageServices: PackageServices{GitAuth: &GitMultiAuth{HTTP: httpAuth, SSH: sshAuth}}}

		gotHTTP := pcx.authForRemote("https://github.com/owner/private-repo", false)
		assert.Equal(t, httpAuth, gotHTTP)

		gotSSH := pcx.authForRemote("git@github.com:owner/private-repo", true)
		assert.Equal(t, sshAuth, gotSSH)
	})
}
