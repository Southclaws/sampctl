package commands

import (
	"context"
	"errors"
	"fmt"
	"io"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
)

func reportRunCommandError(c *cli.Context, err error) error {
	if !isRunCancellation(err) {
		return err
	}

	message := "run stopped\n"
	if c.Bool("verbose") || c.GlobalBool("verbose") {
		message += "details: " + redactDiagnosticText(err.Error()) + "\n"
	}

	if _, writeErr := io.WriteString(c.App.Writer, message); writeErr != nil {
		return &commandExitError{
			code:    1,
			err:     errors.Join(err, fmt.Errorf("write run status: %w", writeErr)),
			message: "could not print the run status",
		}
	}

	return &commandExitError{code: 130, err: err, reported: true}
}

func isRunCancellation(err error) bool {
	if !errors.Is(err, context.Canceled) {
		return false
	}

	switch failure := err.(type) {
	case *compiler.InstallationError:
		return !failure.Committed && !failure.RecoveryPending && isRunCancellation(failure.Err)
	case *runtime.RuntimeInstallError:
		return !failure.Committed && !failure.RecoveryPending && isRunCancellation(failure.Err)
	case *pkgcontext.OperationError:
		if failure.Operation == "recovery" || failure.Outcome == pkgcontext.OutcomeRollbackIncomplete || failure.Outcome == pkgcontext.OutcomeCommitted {
			return false
		}
		return isRunCancellation(failure.Err)
	case interface{ Unwrap() []error }:
		for _, cause := range failure.Unwrap() {
			if cause != nil && !isRunCancellation(cause) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return isRunCancellation(failure.Unwrap())
	default:
		return true
	}
}
