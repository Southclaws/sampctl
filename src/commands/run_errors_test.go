package commands

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
)

func TestReportRunCommandInterruption(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		globalVerbose bool
		localVerbose  bool
	}{
		{name: "normal output"},
		{name: "global verbose", globalVerbose: true},
		{name: "local verbose", localVerbose: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			app := cli.NewApp()
			app.Writer = &output
			global := flag.NewFlagSet("sampctl", flag.ContinueOnError)
			global.Bool("verbose", test.globalVerbose, "")
			local := flag.NewFlagSet("run", flag.ContinueOnError)
			local.Bool("verbose", test.localVerbose, "")
			c := cli.NewContext(app, local, cli.NewContext(app, global, nil))

			cause := fmt.Errorf("failed to run package: received runtime error: received signal: interrupt: %w", context.Canceled)
			err := reportRunCommandError(c, cause)

			assert.ErrorIs(t, err, cause)
			assert.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, 130, ExitCode(err))
			assert.Empty(t, err.Error(), "main must not print a second error")
			want := "run stopped\n"
			if test.globalVerbose || test.localVerbose {
				want += "details: " + cause.Error() + "\n"
			}
			assert.Equal(t, want, output.String())
		})
	}
}

func TestReportRunCommandPreservesFailures(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{
		nil,
		fmt.Errorf("failed to prepare package: %w", context.DeadlineExceeded),
		errors.New("failed to start server"),
		errors.New("failed to write runtime output"),
		errors.New("tests failed"),
		errors.New("received signal: interrupt"),
		errors.Join(context.Canceled, errors.New("failed to stop container")),
		fmt.Errorf("failed to prepare package: %w", &runtime.RuntimeInstallError{
			RecoveryPending: true,
			Err:             errors.Join(context.Canceled, errors.New("rollback failed")),
		}),
		&runtime.RuntimeInstallError{RecoveryPending: true, Err: context.Canceled},
		&runtime.RuntimeInstallError{Committed: true, Err: context.Canceled},
		&compiler.InstallationError{RecoveryPending: true, Err: context.Canceled},
		&compiler.InstallationError{Committed: true, Err: context.Canceled},
		&pkgcontext.OperationError{Operation: "recovery", Err: context.Canceled},
		&pkgcontext.OperationError{Outcome: pkgcontext.OutcomeRollbackIncomplete, Err: context.Canceled},
		&pkgcontext.OperationError{Outcome: pkgcontext.OutcomeCommitted, Err: context.Canceled},
	} {
		var output bytes.Buffer
		app := cli.NewApp()
		app.Writer = &output
		c := cli.NewContext(app, flag.NewFlagSet("run", flag.ContinueOnError), nil)

		assert.Equal(t, cause, reportRunCommandError(c, cause))
		assert.Empty(t, output.String())
	}
}

func TestReportRunCommandCancellationAfterRollback(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{
		errors.Join(context.Canceled, fmt.Errorf("stopped: %w", context.Canceled)),
		&runtime.RuntimeInstallError{Restored: true, Err: errors.Join(context.Canceled)},
		&compiler.InstallationError{Restored: true, Err: context.Canceled},
		&pkgcontext.OperationError{Outcome: pkgcontext.OutcomeRolledBack, Err: context.Canceled},
	} {
		var output bytes.Buffer
		app := cli.NewApp()
		app.Writer = &output
		c := cli.NewContext(app, flag.NewFlagSet("run", flag.ContinueOnError), nil)

		err := reportRunCommandError(c, cause)
		assert.ErrorIs(t, err, cause)
		assert.Equal(t, 130, ExitCode(err))
		assert.Equal(t, "run stopped\n", output.String())
	}
}

func TestReportRunCommandRedactsVerboseDetails(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	app := cli.NewApp()
	app.Writer = &output
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.Bool("verbose", true, "")
	c := cli.NewContext(app, flags, nil)
	cause := fmt.Errorf("download https://person:private@example.com/file?token=private-query: %w", context.Canceled)

	err := reportRunCommandError(c, cause)
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, output.String(), "details: ")
	assert.Contains(t, output.String(), "[redacted]")
	assert.NotContains(t, output.String(), "private")
}

func TestReportRunCommandOutputFailure(t *testing.T) {
	t.Parallel()

	app := cli.NewApp()
	app.Writer = failingDiagnosticWriter{}
	c := cli.NewContext(app, flag.NewFlagSet("run", flag.ContinueOnError), nil)

	err := reportRunCommandError(c, context.Canceled)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Equal(t, 1, ExitCode(err))
	assert.EqualError(t, err, "could not print the run status")
}
