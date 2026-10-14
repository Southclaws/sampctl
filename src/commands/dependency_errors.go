package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func reportDependencyCommandError(c *cli.Context, operation string, err error) error {
	if err == nil {
		return nil
	}

	var reported *commandExitError
	if errors.As(err, &reported) && reported.reported {
		return err
	}

	verbose := c.Bool("verbose") || c.GlobalBool("verbose")
	style := newCheckOutputStyle(c.App.Writer, c.Bool("bare") || c.GlobalBool("bare"))
	var output bytes.Buffer
	outcome := dependencyOperationOutcome(err)
	recoveryFailed := dependencyOperationRecoveryFailed(err)
	status := checkFailed
	if outcome == pkgcontext.OutcomeCommitted && !recoveryFailed {
		status = checkPassed
	}

	writeCheckResult(&output, diagnostic{Status: status, Message: dependencyFailureHeading(operation, err)}, false, style)
	for _, result := range dependencyFailureDiagnostics(err) {
		if outcome == pkgcontext.OutcomeCommitted && recoveryFailed && result.Name == "recovery" {
			result.Action = "Run `sampctl check --verbose`. Keep the backups and recovery files until recovery finishes."
		}

		if outcome == pkgcontext.OutcomeRollbackIncomplete && result.Name == "rollback" {
			result.Action = "Run `sampctl check`. Keep the backups until your dependencies are restored."
		}

		if status == checkPassed {
			result.Status = checkWarning
			if result.Name == "cleanup" {
				result.Action = "Your changes were saved. Run `sampctl check` and use `sampctl check --fix` if it offers cleanup."
			}
		}

		writeCheckResult(&output, result, verbose, style)
	}
	if !verbose {
		output.WriteString("Use --verbose for details.\n")
	}

	text := output.String()
	if state, stateErr := getCommandState(c); stateErr == nil && state.cfg != nil {
		for _, secret := range []string{state.cfg.GitHubToken, state.cfg.GitPassword} {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, "[redacted]")
			}
		}
	}

	if _, writeErr := c.App.Writer.Write([]byte(text)); writeErr != nil {
		message := "Cannot print dependency errors."
		if status == checkPassed {
			message = "Your changes were saved, but the cleanup error could not be printed."
		}
		return &commandExitError{code: 1, err: errors.Join(err, fmt.Errorf("write dependency diagnostics: %w", writeErr)), message: message}
	}

	return &commandExitError{code: 1, err: err, reported: true}
}

func dependencyFailureHeading(operation string, err error) string {
	outcome := dependencyOperationOutcome(err)
	if outcome == pkgcontext.OutcomeCommitted {
		if dependencyOperationRecoveryFailed(err) {
			return "Previous dependency changes were saved. Recovery failed"
		}
		return "Dependency changes saved. Cleanup pending"
	}

	var message string
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		message = "Dependency command cancelled"
		if operation == "build" {
			message = "Build cancelled"
		}
	} else {
		switch operation {
		case "install":
			message = "Dependency installation failed"
		case "uninstall":
			message = "Dependency removal failed"
		case "build":
			message = "Build failed"
		default:
			message = "Dependency setup failed"
		}
	}

	switch outcome {
	case pkgcontext.OutcomeRolledBack:
		return message + ". Previous dependency state restored"
	case pkgcontext.OutcomeRollbackIncomplete:
		return message + ". Recovery is incomplete"
	default:
		return message
	}
}

func dependencyOperationRecoveryFailed(err error) bool {
	switch failure := err.(type) {
	case *pkgcontext.OperationError:
		return failure.Operation == "recovery" || dependencyOperationRecoveryFailed(failure.Err)
	case interface{ Unwrap() []error }:
		for _, cause := range failure.Unwrap() {
			if dependencyOperationRecoveryFailed(cause) {
				return true
			}
		}

		return false
	case interface{ Unwrap() error }:
		return dependencyOperationRecoveryFailed(failure.Unwrap())
	default:
		return false
	}
}

func dependencyOperationOutcome(err error) pkgcontext.DependencyOperationOutcome {
	switch failure := err.(type) {
	case *pkgcontext.OperationError:
		if failure.Outcome != "" {
			return failure.Outcome
		}

		return dependencyOperationOutcome(failure.Err)
	case interface{ Unwrap() []error }:
		var outcome pkgcontext.DependencyOperationOutcome
		for _, cause := range failure.Unwrap() {
			current := dependencyOperationOutcome(cause)
			if current == pkgcontext.OutcomeRollbackIncomplete {
				return current
			}
			if current == pkgcontext.OutcomeCommitted || outcome == "" {
				outcome = current
			}
		}

		return outcome
	case interface{ Unwrap() error }:
		return dependencyOperationOutcome(failure.Unwrap())
	default:
		return ""
	}
}
