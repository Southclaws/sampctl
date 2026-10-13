package commands

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
)

func TestReportInstallationOutcomes(t *testing.T) {
	t.Parallel()
	cause := errors.New("technical installation detail")
	for _, test := range []struct {
		name   string
		err    error
		want   string
		absent string
	}{
		{"runtime restored", &runtimepkg.RuntimeInstallError{Runtime: "openmp", Restored: true, Err: cause}, "Previous runtime state restored", "Cleanup pending"},
		{"runtime committed", &runtimepkg.RuntimeInstallError{Runtime: "openmp", Committed: true, RecoveryPending: true, Err: cause}, "Runtime openmp installed. Cleanup pending", "Previous runtime state restored"},
		{"runtime committed and clean", &runtimepkg.RuntimeInstallError{Runtime: "openmp", Committed: true, Err: cause}, "Runtime openmp installed", "Cleanup pending"},
		{"runtime recovery needed", &runtimepkg.RuntimeInstallError{Runtime: "openmp", RecoveryPending: true, Err: cause}, "follow its recovery instructions", "Previous runtime state restored"},
		{"compiler restored", &compiler.InstallationError{Path: "/cache/compiler/v3", Restored: true, Err: cause}, "Previous compiler restored", "Cleanup pending"},
		{"compiler committed", &compiler.InstallationError{Path: "/cache/compiler/v3", Committed: true, RecoveryPending: true, Err: cause}, "Compiler installation saved. Cleanup pending", "Previous compiler restored"},
		{"compiler committed and clean", &compiler.InstallationError{Path: "/cache/compiler/v3", Committed: true, Err: cause}, "Compiler installation saved", "Cleanup pending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := newTestCLIContext("master")
			var output bytes.Buffer
			ctx.App.Writer = &output
			failure := &pkgcontext.DependencyError{Dependency: "runtime openmp", Operation: "ensure binaries", Err: test.err}
			if _, compilerFailure := test.err.(*compiler.InstallationError); compilerFailure {
				failure = nil
			}
			err := test.err
			if failure != nil {
				err = failure
			}
			err = reportDependencyCommandError(ctx, "build", fmt.Errorf("prepare: %w", err))
			if !errors.Is(err, cause) || ExitCode(err) != 1 {
				t.Errorf("reportDependencyCommandError(%s) = %v, want preserved cause and exit 1", test.name, err)
			}
			if !strings.Contains(output.String(), test.want) || strings.Contains(output.String(), test.absent) || strings.Contains(output.String(), cause.Error()) {
				t.Errorf("reportDependencyCommandError(%s) output = %q, want %q without %q or raw details", test.name, output.String(), test.want, test.absent)
			}
		})
	}
}

func TestReportCompilerExecutionKeepsDetailsForVerbose(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{compiler.ExecutionMissing, compiler.ExecutionStart, compiler.ExecutionCrash, compiler.ExecutionOutput, compiler.ExecutionCancelled} {
		for _, verbose := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/verbose=%t", kind, verbose), func(t *testing.T) {
				ctx := newTestCLIContext("master")
				set := flag.NewFlagSet("test", flag.ContinueOnError)
				set.Bool("verbose", verbose, "")
				ctx = cli.NewContext(ctx.App, set, nil)
				var output bytes.Buffer
				ctx.App.Writer = &output
				cause := errors.New("technical compiler detail")
				failure := &compiler.ExecutionError{Kind: kind, Path: "/cache/compiler/pawncc", Output: "internal compiler output\nGITHUB_TOKEN=private-value", Err: cause}
				err := reportDependencyCommandError(ctx, "build", failure)
				if !errors.Is(err, cause) || ExitCode(err) != 1 {
					t.Errorf("reportDependencyCommandError(%s) = %v, want preserved cause and exit 1", kind, err)
				}
				text := output.String()
				if strings.Contains(text, "technical compiler detail") != verbose || strings.Contains(text, "internal compiler output") != verbose || strings.Contains(text, "private-value") {
					t.Errorf("reportDependencyCommandError(%s, verbose=%t) output = %q, want verbose-only redacted details", kind, verbose, text)
				}
				if strings.Contains(text, "Dependency command failed") || strings.Contains(text, ";") {
					t.Errorf("reportDependencyCommandError(%s) output = %q, want compiler-specific plain text", kind, text)
				}
			})
		}
	}
}
