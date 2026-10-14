package commands

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestReportDependencyDestinationConflict(t *testing.T) {
	t.Parallel()

	for _, verbose := range []bool{false, true} {
		t.Run(fmt.Sprintf("verbose=%t", verbose), func(t *testing.T) {
			ctx := newTestCLIContext("dev")
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.Bool("verbose", verbose, "")
			ctx = cli.NewContext(ctx.App, set, nil)
			var output bytes.Buffer
			ctx.App.Writer = &output

			conflict := &pkgcontext.DependencyDestinationConflictError{
				First:     versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "stdlib", Tag: "v1"},
				Second:    versioning.DependencyMeta{Site: "github.com", User: "other", Repo: "stdlib", Branch: "main"},
				Directory: "stdlib",
			}
			dependency := &pkgcontext.DependencyError{Dependency: conflict.Second.String(), Operation: "ensure", Err: fmt.Errorf("validate destinations: %w", conflict)}
			failure := &pkgcontext.OperationError{
				Operation: "dependency operation", Outcome: pkgcontext.OutcomeRolledBack,
				Err: errors.Join(dependency, dependency),
			}

			err := reportDependencyCommandError(ctx, "install", failure)
			var got *pkgcontext.DependencyDestinationConflictError
			if !errors.As(err, &got) || got != conflict || ExitCode(err) != 1 {
				t.Errorf("conflict report = %v, want preserved conflict and exit 1", err)
			}

			for _, want := range []string{conflict.First.String(), conflict.Second.String(), `both install to "stdlib"`, "one repository and version", "Previous dependency state restored"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("conflict output = %q, want %q", output.String(), want)
				}
			}

			if strings.Contains(output.String(), "Dependency command failed") {
				t.Errorf("conflict output hides the conflicting dependencies: %s", output.String())
			}
			if got := strings.Count(output.String(), `both install to "stdlib"`); got != 1 {
				t.Errorf("conflict diagnostics = %d, want one: %s", got, output.String())
			}

			if gotDetails := strings.Contains(output.String(), "Details:"); gotDetails != verbose {
				t.Errorf("verbose=%t: details = %t", verbose, gotDetails)
			}
		})
	}
}

func TestReportDependencyFailureDuplicates(t *testing.T) {
	t.Parallel()

	ctx := newTestCLIContext("dev")
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.Bool("verbose", true, "")
	ctx = cli.NewContext(ctx.App, set, nil)
	var output bytes.Buffer
	ctx.App.Writer = &output

	first := errors.New("download failed")
	second := errors.New("invalid archive")
	failure := errors.Join(
		&pkgcontext.DependencyError{Dependency: "fixture/repo", Operation: "ensure", Err: first},
		&pkgcontext.DependencyError{Dependency: "fixture/repo", Operation: "ensure", Err: first},
		&pkgcontext.DependencyError{Dependency: "fixture/repo", Operation: "ensure", Err: second},
		&pkgcontext.OperationError{Operation: "rollback", Err: os.ErrPermission},
		&pkgcontext.OperationError{Operation: "cleanup", Err: os.ErrPermission},
	)

	err := reportDependencyCommandError(ctx, "install", failure)
	for _, cause := range []error{first, second, os.ErrPermission} {
		if !errors.Is(err, cause) {
			t.Errorf("duplicate report lost cause %v: %v", cause, err)
		}
	}

	if got := strings.Count(output.String(), "✗ fixture/repo: Dependency command failed"); got != 2 {
		t.Errorf("dependency diagnostics = %d, want one for each distinct cause: %s", got, output.String())
	}

	for _, want := range []string{"download failed", "invalid archive", "Could not restore all previous dependencies", "Cannot remove temporary dependency files"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("duplicate report lost %q: %s", want, output.String())
		}
	}
}

func TestDependencyDiagnosticsPreserveDifferentCauseTypes(t *testing.T) {
	t.Parallel()

	failure := errors.Join(
		&pkgcontext.DependencyError{Dependency: "fixture/repo", Operation: "ensure", Err: fmt.Errorf("validate: %w", errors.New("fixture failure"))},
		&pkgcontext.DependencyError{Dependency: "fixture/repo", Operation: "ensure", Err: fmt.Errorf("validate: %w", diagnosticFixtureError("fixture failure"))},
	)

	if got := len(dependencyFailureDiagnostics(failure)); got != 2 {
		t.Errorf("diagnostics = %d, want both distinct causes", got)
	}
}

type diagnosticFixtureError string

func (err diagnosticFixtureError) Error() string { return string(err) }
