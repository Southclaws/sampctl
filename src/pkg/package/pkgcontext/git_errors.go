package pkgcontext

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// WrapGitError provides concise, helpful error messages for Git operations
func WrapGitError(err error, meta versioning.DependencyMeta) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	dependency := fmt.Sprintf("%s/%s", meta.User, meta.Repo)

	// Handle common Git errors with brief, helpful messages
	switch {
	case strings.Contains(errStr, "authentication required"):
		return fmt.Errorf("dependency '%s': authentication required (repo may be private or not exist)", dependency)

	case strings.Contains(errStr, "repository not found") ||
		strings.Contains(errStr, "not found"):
		return fmt.Errorf("dependency '%s': repository not found (check URL or may be private)", dependency)

	case strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout"):
		return fmt.Errorf("dependency '%s': network error - check internet connection", dependency)

	case strings.Contains(errStr, "remote: Repository access blocked"):
		return fmt.Errorf("dependency '%s': repository access blocked", dependency)

	case err == git.ErrRepositoryNotExists:
		return fmt.Errorf("dependency '%s': repository does not exist locally (will be cloned)", dependency)

	default:
		// For unknown errors, provide the original error with context
		return fmt.Errorf("dependency '%s': %v", dependency, err)
	}
}
