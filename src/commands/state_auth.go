package commands

import (
	"github.com/go-git/go-git/v5/plumbing/transport"
	transporthttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	transportssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func buildGitAuth(cfg *config.Config) transport.AuthMethod {
	sshAuth, err := transportssh.DefaultAuthBuilder("git")
	if err != nil {
		print.Verb("Failed to set up SSH:", err)
	}

	return buildGitAuthWithSSH(cfg, sshAuth)
}

func buildGitAuthWithSSH(cfg *config.Config, sshAuth transport.AuthMethod) transport.AuthMethod {
	httpAuth := newHTTPAuth(cfg)

	switch {
	case httpAuth != nil && sshAuth != nil:
		return &pkgcontext.GitMultiAuth{HTTP: httpAuth, SSH: sshAuth}
	case httpAuth != nil:
		return httpAuth
	default:
		return sshAuth
	}
}

func newHTTPAuth(cfg *config.Config) transport.AuthMethod {
	if cfg == nil {
		return nil
	}

	switch {
	case cfg.GitUsername != "" && cfg.GitPassword != "":
		return &transporthttp.BasicAuth{
			Username: cfg.GitUsername,
			Password: cfg.GitPassword,
		}
	case cfg.GitHubToken != "":
		return &transporthttp.BasicAuth{
			Username: "x-access-token",
			Password: cfg.GitHubToken,
		}
	default:
		return nil
	}
}
