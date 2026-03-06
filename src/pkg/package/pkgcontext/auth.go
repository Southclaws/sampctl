package pkgcontext

import "github.com/go-git/go-git/v5/plumbing/transport"

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
