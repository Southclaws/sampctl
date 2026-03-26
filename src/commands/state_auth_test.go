package commands

import (
	"testing"

	transporthttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestNewHTTPAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *config.Config
		want *transporthttp.BasicAuth
	}{
		{
			name: "username and password",
			cfg:  &config.Config{GitUsername: "alice", GitPassword: "secret", GitHubToken: "token"},
			want: &transporthttp.BasicAuth{Username: "alice", Password: "secret"},
		},
		{
			name: "github token fallback",
			cfg:  &config.Config{GitHubToken: "token"},
			want: &transporthttp.BasicAuth{Username: "x-access-token", Password: "token"},
		},
		{
			name: "no credentials",
			cfg:  &config.Config{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := newHTTPAuth(tt.cfg)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}

			require.IsType(t, &transporthttp.BasicAuth{}, got)
			assert.Equal(t, tt.want, got.(*transporthttp.BasicAuth))
		})
	}
}

func TestBuildGitAuthPrefersHTTPWhenSSHUnavailable(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{GitHubToken: "token"}
	auth := buildGitAuthWithSSH(cfg, nil)

	require.IsType(t, &transporthttp.BasicAuth{}, auth)
	assert.Equal(t, &transporthttp.BasicAuth{Username: "x-access-token", Password: "token"}, auth.(*transporthttp.BasicAuth))
}

func TestBuildGitAuthCombinesHTTPAndSSH(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{GitHubToken: "token"}
	sshAuth := &transporthttp.BasicAuth{Username: "ssh", Password: "secret"}
	auth := buildGitAuthWithSSH(cfg, sshAuth)

	require.IsType(t, &pkgcontext.GitMultiAuth{}, auth)
	multi := auth.(*pkgcontext.GitMultiAuth)
	assert.Equal(t, &transporthttp.BasicAuth{Username: "x-access-token", Password: "token"}, multi.HTTP)
	assert.Equal(t, sshAuth, multi.SSH)
}
