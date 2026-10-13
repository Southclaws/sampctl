package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type gitOperationError struct {
	message string
	cause   error
}

func (err *gitOperationError) Error() string { return err.message }
func (err *gitOperationError) Unwrap() error { return err.cause }

// WrapGitError provides concise Git diagnostics and preserves the underlying error.
func WrapGitError(err error, meta versioning.DependencyMeta) error {
	if err == nil {
		return nil
	}

	message := "repository operation failed. Use --verbose for details"
	errText := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		message = "operation canceled"
	case errors.Is(err, context.DeadlineExceeded):
		message = "operation timed out"
	case errors.Is(err, transport.ErrInvalidAuthMethod):
		message = "Git authentication method does not match the repository transport. Use --verbose for details"
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed), strings.Contains(errText, "authentication required"):
		message = "authentication required. Check your Git credentials and repository access"
	case errors.Is(err, transport.ErrRepositoryNotFound), strings.Contains(errText, "repository not found"):
		message = "repository not found. Check the repository name and your Git credentials"
	case errors.Is(err, os.ErrPermission):
		message = "permission denied. Check access to the dependency and cache directories"
	case errors.Is(err, os.ErrInvalid):
		message = "unsafe repository directory. Check the dependency and cache directories for symbolic links or unmanaged files"
	case errors.Is(err, syscall.ENOSPC):
		message = "not enough disk space. Free space in the dependency and cache directories"
	case isTransientGitError(err):
		message = "network error. Check your internet connection and try again"
	case errors.Is(err, git.ErrRepositoryNotExists):
		message = "repository does not exist locally"
	case strings.Contains(errText, "repository access blocked"):
		message = "repository access blocked. Check your repository access"
	}
	return &gitOperationError{message: fmt.Sprintf("dependency '%s/%s': %s", meta.User, meta.Repo, message), cause: err}
}

func isTransientGitError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var pathErr *os.PathError
	var permanent *plumbing.PermanentError
	if errors.As(err, &pathErr) || errors.As(err, &permanent) {
		return false
	}
	var unexpected *plumbing.UnexpectedError
	if errors.As(err, &unexpected) {
		return isTransientGitError(unexpected.Err)
	}
	var responseErr *gitHTTP.Err
	if errors.As(err, &responseErr) && responseErr.Response != nil {
		switch responseErr.Response.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	if dnsErr != nil && dnsErr.IsTemporary {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT)
}
