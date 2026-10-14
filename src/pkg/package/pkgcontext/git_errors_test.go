package pkgcontext

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestWrapGitErrorDoesNotRecommendCredentialsForOtherFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		cause error
	}{
		{name: "unknown failure", cause: errors.New("unexpected repository response from https://user:secret@example.com/repo")},
		{name: "transport mismatch", cause: fmt.Errorf("failed to update repository: %w", transport.ErrInvalidAuthMethod)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := WrapGitError(test.cause, versioning.DependencyMeta{User: "fixture", Repo: "public"})
			if !errors.Is(err, test.cause) {
				t.Errorf("WrapGitError(%v) = %v, want preserved cause", test.cause, err)
			}

			if !strings.Contains(err.Error(), "fixture/public") || !strings.Contains(err.Error(), "--verbose") {
				t.Errorf("WrapGitError(%v) = %q, want dependency context and verbose advice", test.cause, err)
			}

			if strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "secret") {
				t.Errorf("WrapGitError(%v) = %q, want no credential advice or sensitive details", test.cause, err)
			}
		})
	}
}
