package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestReportCommittedRecoveryFailure(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"changed manifest", "missing manifest"} {
		t.Run(scenario, func(t *testing.T) {
			root, stage := newCheckRecoveryFixture(t, "committed")
			manifest := filepath.Join(root, "pawn.json")
			if scenario == "changed manifest" {
				writeCheckRecoveryFixture(t, manifest, []byte(`{"user":"manual edit"}`))
			} else if err := os.Remove(manifest); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(root, pkgcontext.DependencyRecoveryFilename)
			beforeJournal := readCheckRecoveryFixture(t, journal)
			state, cause := pkgcontext.RecoverDependencyOperation(context.Background(), root)
			if cause == nil || !state.Committed {
				t.Fatalf("recover inconsistent committed state = %+v, %v; want committed state and an error", state, cause)
			}
			ctx := newTestCLIContext("dev")
			var output bytes.Buffer
			ctx.App.Writer = &output
			err := reportDependencyCommandError(ctx, "ensure", &pkgcontext.OperationError{Operation: "recovery", Err: cause})
			if !errors.Is(err, cause) || ExitCode(err) != 1 {
				t.Errorf("recovery report = %v, code = %d; want preserved cause and nonzero exit", err, ExitCode(err))
			}
			var operation *pkgcontext.OperationError
			if !errors.As(cause, &operation) || operation.Outcome != pkgcontext.OutcomeCommitted {
				t.Errorf("recovery outcome = %+v; want original committed outcome", operation)
			}
			assertCommittedRecoveryOutput(t, output.String())
			if !bytes.Equal(beforeJournal, readCheckRecoveryFixture(t, journal)) {
				t.Error("invalid recovery changed its journal")
			}
			if _, err := os.Stat(stage); err != nil {
				t.Errorf("invalid recovery lost its staged files: %v", err)
			}
			if scenario == "changed manifest" {
				if got := string(readCheckRecoveryFixture(t, manifest)); got != `{"user":"manual edit"}` {
					t.Errorf("invalid recovery changed the manifest: %s", got)
				}
			} else if _, err := os.Lstat(manifest); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("invalid committed recovery restored the missing manifest: %v", err)
			}
		})
	}
}

func TestReportCommittedRecoveryWrappers(t *testing.T) {
	t.Parallel()
	cause := &pkgcontext.OperationError{Operation: "recovery", Outcome: pkgcontext.OutcomeCommitted, Err: os.ErrPermission}
	for _, failure := range []error{
		cause,
		fmt.Errorf("ensure: %w", cause),
		errors.Join(errors.New("another recovery error"), cause),
		&pkgcontext.OperationError{Operation: "project", Err: cause},
	} {
		ctx := newTestCLIContext("dev")
		var output bytes.Buffer
		ctx.App.Writer = &output
		err := reportDependencyCommandError(ctx, "ensure", failure)
		if !errors.Is(err, os.ErrPermission) || ExitCode(err) != 1 {
			t.Errorf("wrapped recovery report = %v, code = %d; want preserved cause and nonzero exit", err, ExitCode(err))
		}
		assertCommittedRecoveryOutput(t, output.String())
	}
}

func assertCommittedRecoveryOutput(t *testing.T, output string) {
	t.Helper()
	if !strings.Contains(output, "✗ Previous dependency changes were saved; recovery could not complete") {
		t.Errorf("recovery output = %s; want a failed recovery heading that preserves prior commit status", output)
	}
	if !strings.Contains(output, "Keep backups and recovery metadata") {
		t.Errorf("recovery output = %s; want instructions to preserve recovery evidence", output)
	}
	for _, forbidden := range []string{"✓", "operation completed", "cleanup is pending", "state restored", "sampctl check --fix"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("invalid committed recovery output contains %q: %s", forbidden, output)
		}
	}
}
