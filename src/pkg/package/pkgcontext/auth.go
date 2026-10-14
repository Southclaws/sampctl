package pkgcontext

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// GitMultiAuth stores separate auth methods for HTTPS and SSH remotes.
type GitMultiAuth struct {
	HTTP transport.AuthMethod
	SSH  transport.AuthMethod
}

// Name satisfies transport.AuthMethod.
func (a *GitMultiAuth) Name() string {
	return "multi-auth"
}

// String satisfies transport.AuthMethod.
func (a *GitMultiAuth) String() string {
	return a.Name()
}

func (pcx PackageContext) shouldRetryWithSSH(meta versioning.DependencyMeta, from string, ssh bool, err error) bool {
	if err == nil || ssh || pcx.GitAuth == nil || stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if !strings.HasPrefix(from, "https://") && !strings.HasPrefix(from, "http://") {
		return false
	}

	site := meta.Site
	if site == "" {
		site = "github.com"
	}
	if site != "github.com" {
		return false
	}

	if meta.User == "" || meta.Repo == "" {
		return false
	}

	if pcx.authForRemote(toGitSSHURL(meta), true) == nil {
		return false
	}

	return isGitAuthenticationError(err)
}

func toGitSSHURL(meta versioning.DependencyMeta) string {
	site := meta.Site
	if site == "" {
		site = "github.com"
	}

	user := meta.SSH
	if user == "" {
		user = "git"
	}

	return fmt.Sprintf("%s@%s:%s/%s", user, site, meta.User, meta.Repo)
}

func isGitAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "authentication required") ||
		strings.Contains(errStr, "repository not found") ||
		strings.Contains(errStr, "could not read from remote repository") ||
		strings.Contains(errStr, "permission denied")
}

func (pcx PackageContext) authForRemote(from string, ssh bool) transport.AuthMethod {
	if pcx.GitAuth == nil {
		return nil
	}

	if authBundle, ok := pcx.GitAuth.(*GitMultiAuth); ok {
		if ssh || isSSHRemote(from) {
			if authBundle.SSH != nil {
				return authBundle.SSH
			}
			return nil
		}

		if strings.HasPrefix(from, "https://") || strings.HasPrefix(from, "http://") {
			if authBundle.HTTP != nil {
				return authBundle.HTTP
			}
			return nil
		}

		return nil
	}

	authName := pcx.GitAuth.Name()

	if ssh || isSSHRemote(from) {
		if strings.HasPrefix(authName, "ssh-") {
			return pcx.GitAuth
		}
		return nil
	}

	if strings.HasPrefix(from, "https://") || strings.HasPrefix(from, "http://") {
		if strings.HasPrefix(authName, "http-") {
			return pcx.GitAuth
		}
		return nil
	}

	return nil
}

func isSSHRemote(from string) bool {
	if strings.HasPrefix(from, "ssh://") || strings.HasPrefix(from, "git@") {
		return true
	}
	return strings.Contains(from, "@") && strings.Contains(from, ":")
}
