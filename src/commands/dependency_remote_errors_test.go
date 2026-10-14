package commands

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/github"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
)

func TestReportDependencyGitHubRateLimit(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{60, 5000} {
		for _, verbose := range []bool{false, true} {
			t.Run(fmt.Sprintf("limit=%d/verbose=%t", limit, verbose), func(t *testing.T) {
				ctx := newTestCLIContext("dev")
				set := flag.NewFlagSet("test", flag.ContinueOnError)
				set.Bool("verbose", verbose, "")
				ctx = cli.NewContext(ctx.App, set, nil)
				var output bytes.Buffer
				ctx.App.Writer = &output

				request, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/fixture/plugin/tags", nil)
				if err != nil {
					t.Fatal(err)
				}

				reset := time.Date(2026, time.October, 13, 18, 0, 0, 0, time.FixedZone("offset", 3600))
				cause := &github.RateLimitError{
					Rate:     github.Rate{Limit: limit, Reset: github.Timestamp{Time: reset}},
					Response: &http.Response{StatusCode: http.StatusForbidden, Request: request},
					Message:  "API rate limit exceeded",
				}
				failure := &pkgcontext.DependencyError{
					Dependency: "fixture/plugin", Operation: "resolve tags", Err: fmt.Errorf("request tags: %w", cause),
				}

				err = reportDependencyCommandError(ctx, "install", failure)
				var got *github.RateLimitError
				if !errors.As(err, &got) || got != cause || ExitCode(err) != 1 {
					t.Errorf("reportDependencyCommandError(rate limit) = %v, want preserved rate limit and exit 1", err)
				}

				for _, want := range []string{"fixture/plugin: GitHub API rate limit reached", "Retry after 2026-10-13 17:00:00 UTC."} {
					if !strings.Contains(output.String(), want) {
						t.Errorf("reportDependencyCommandError(rate limit) output = %q, want %q", output.String(), want)
					}
				}

				if hasTokenAdvice := strings.Contains(output.String(), "Set SAMPCTL_GITHUB_TOKEN"); hasTokenAdvice != (limit == 60) {
					t.Errorf("reportDependencyCommandError(limit=%d) token advice = %t, want %t", limit, hasTokenAdvice, limit == 60)
				}
				if strings.Contains(output.String(), "Git credentials") || strings.Contains(output.String(), "Dependency command failed") {
					t.Errorf("reportDependencyCommandError(rate limit) output = %q, want rate limit advice", output.String())
				}

				if hasDetails := strings.Contains(output.String(), "Details:"); hasDetails != verbose {
					t.Errorf("reportDependencyCommandError(verbose=%t) details = %t, want %t", verbose, hasDetails, verbose)
				}
			})
		}
	}
}

func TestDependencyRateLimitAdviceWithoutReset(t *testing.T) {
	t.Parallel()

	message, action := dependencyFailureAdvice(&github.RateLimitError{})
	if message != "GitHub API rate limit reached" || action != "Wait for GitHub's rate limit to reset, then retry." {
		t.Errorf("dependencyFailureAdvice(rate limit without reset) = %q, %q, want rate limit and wait advice", message, action)
	}
}

func TestReportDependencyUnsupportedPluginRuntime(t *testing.T) {
	t.Parallel()

	ctx := newTestCLIContext("dev")
	var output bytes.Buffer
	ctx.App.Writer = &output

	cause := &runtime.PluginResourceNotFoundError{
		Meta: versioning.DependencyMeta{User: "IS4Code", Repo: "YSF", Tag: "v2.2"}, Platform: "linux", Version: "openmp",
	}
	failure := &pkgcontext.OperationError{
		Operation: "dependency operation", Outcome: pkgcontext.OutcomeRolledBack,
		Err: &pkgcontext.DependencyError{
			Dependency: "runtime openmp", Operation: "ensure plugins", Err: fmt.Errorf("get plugin from cache: %w", cause),
		},
	}

	err := reportDependencyCommandError(ctx, "install", failure)
	if !errors.Is(err, cause) || ExitCode(err) != 1 {
		t.Errorf("reportDependencyCommandError(unsupported plugin runtime) = %v, want preserved failure and exit 1", err)
	}

	for _, want := range []string{"Previous dependency state restored", "IS4Code/YSF:v2.2: No plugin resource for linux and runtime openmp", "Choose a package or release that supports this platform and runtime"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("reportDependencyCommandError(unsupported plugin runtime) output = %q, want %q", output.String(), want)
		}
	}

	if strings.Contains(output.String(), "runtime openmp: Dependency command failed") {
		t.Errorf("reportDependencyCommandError(unsupported plugin runtime) output = %q, want package and requested runtime", output.String())
	}
}
