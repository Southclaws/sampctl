package pkgcontext

import (
	"errors"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestShouldRetryWithSSH(t *testing.T) {
	t.Parallel()

	meta := versioning.DependencyMeta{Site: "github.com", User: "owner", Repo: "private-repo"}
	const remote = "https://github.com/owner/private-repo"
	authErr := errors.New("authentication required: Repository not found")
	sshAuth := &ssh.PublicKeys{User: "git"}
	httpAuth := &http.BasicAuth{Username: "u", Password: "p"}

	for _, tc := range []struct {
		name     string
		auth     transport.AuthMethod
		usingSSH bool
		err      error
		want     bool
	}{
		{name: "HTTPS authentication fails with SSH available", auth: sshAuth, err: authErr, want: true},
		{name: "already using SSH", auth: sshAuth, usingSSH: true, err: authErr},
		{name: "network timeout", auth: sshAuth, err: errors.New("network timeout")},
		{name: "HTTP credentials only", auth: httpAuth, err: authErr},
		{
			name: "SSH credentials in auth bundle",
			auth: &GitMultiAuth{HTTP: httpAuth, SSH: sshAuth},
			err:  authErr,
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pcx := PackageContext{PackageServices: PackageServices{GitAuth: tc.auth}}

			got := pcx.shouldRetryWithSSH(meta, remote, tc.usingSSH, tc.err)

			assert.Equal(t, tc.want, got)
		})
	}
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

func TestAuthForRemoteMatchesTransport(t *testing.T) {
	t.Parallel()

	const httpsRemote = "https://github.com/owner/private-repo"
	const sshRemote = "git@github.com:owner/private-repo"
	httpAuth := &http.BasicAuth{Username: "u", Password: "p"}
	sshAuth := &ssh.PublicKeys{User: "git"}
	bundle := &GitMultiAuth{HTTP: httpAuth, SSH: sshAuth}

	for _, tc := range []struct {
		name     string
		auth     transport.AuthMethod
		remote   string
		usingSSH bool
		want     transport.AuthMethod
	}{
		{name: "HTTP credentials for HTTPS", auth: httpAuth, remote: httpsRemote, want: httpAuth},
		{name: "HTTP credentials ignored for SSH", auth: httpAuth, remote: sshRemote, usingSSH: true},
		{name: "SSH credentials for SSH", auth: sshAuth, remote: sshRemote, usingSSH: true, want: sshAuth},
		{name: "SSH credentials ignored for HTTPS", auth: sshAuth, remote: httpsRemote},
		{name: "bundle supplies HTTP credentials", auth: bundle, remote: httpsRemote, want: httpAuth},
		{name: "bundle supplies SSH credentials", auth: bundle, remote: sshRemote, usingSSH: true, want: sshAuth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pcx := PackageContext{PackageServices: PackageServices{GitAuth: tc.auth}}

			got := pcx.authForRemote(tc.remote, tc.usingSSH)

			assert.Equal(t, tc.want, got)
		})
	}
}
