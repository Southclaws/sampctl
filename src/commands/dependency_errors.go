package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"go.yaml.in/yaml/v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
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
			return "Previous dependency changes were saved; recovery failed"
		}
		return "Dependency changes saved; cleanup pending"
	}
	var message string
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		message = "Dependency command cancelled"
	} else {
		switch operation {
		case "install":
			message = "Dependency installation failed"
		case "uninstall":
			message = "Dependency removal failed"
		default:
			message = "Dependency setup failed"
		}
	}
	switch outcome {
	case pkgcontext.OutcomeRolledBack:
		return message + "; previous dependency state restored"
	case pkgcontext.OutcomeRollbackIncomplete:
		return message + "; recovery is incomplete"
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

func dependencyFailureDiagnostics(err error) []diagnostic {
	results := collectDependencyFailureDiagnostics(err, err)
	seen := make(map[dependencyDiagnosticKey]struct{}, len(results))
	unique := results[:0]
	for _, result := range results {
		key := dependencyDiagnosticKey{
			Name: result.Name, Status: result.Status, Message: result.Message,
			Action: result.Action, Path: result.Path, Cause: dependencyDiagnosticCause(result.Err),
			CanFix: result.CanFix, RepairAttempted: result.RepairAttempted, RepairVerified: result.RepairVerified,
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, result)
	}
	return unique
}

type dependencyDiagnosticKey struct {
	Name, Message, Action, Path, Cause      string
	Status                                  checkStatus
	CanFix, RepairAttempted, RepairVerified bool
}

func dependencyDiagnosticCause(err error) string {
	if err == nil {
		return ""
	}
	key := fmt.Sprintf("%T(%q)", err, err.Error())
	switch failure := err.(type) {
	case interface{ Unwrap() []error }:
		for _, cause := range failure.Unwrap() {
			key += "[" + dependencyDiagnosticCause(cause) + "]"
		}
	case interface{ Unwrap() error }:
		key += "[" + dependencyDiagnosticCause(failure.Unwrap()) + "]"
	}
	return key
}

func collectDependencyFailureDiagnostics(err, details error) []diagnostic {
	switch failure := err.(type) {
	case *pkgcontext.DependencyError:
		message, action := dependencyFailureAdvice(failure.Err)
		if failure.Operation == "select" {
			message = "Dependency is not declared in this project"
			action = "Check the dependency name in pawn.json or pawn.yaml, then retry."
		}
		result := diagnostic{Name: "dependencies", Status: checkFailed, Message: failure.Dependency + ": " + message, Action: action, Err: details}
		var missingAsset *download.ReleaseAssetNotFoundError
		if errors.As(failure.Err, &missingAsset) {
			result = releaseAssetFailureDiagnostic(missingAsset, details)
		}
		var conflict *pkgcontext.DependencyDestinationConflictError
		if errors.As(failure.Err, &conflict) {
			result = dependencyDestinationConflictDiagnostic(conflict, details)
		}
		results := []diagnostic{result}
		return append(results, dependencyRecoveryDiagnostics(failure.Err)...)
	case *download.ReleaseAssetNotFoundError:
		return []diagnostic{releaseAssetFailureDiagnostic(failure, details)}
	case *pkgcontext.DependencyDestinationConflictError:
		return []diagnostic{dependencyDestinationConflictDiagnostic(failure, details)}
	case *pkgcontext.OperationError:
		if failure.Outcome != "" || (dependencyOperationOutcome(failure) == pkgcontext.OutcomeCommitted && dependencyOperationRecoveryFailed(failure)) {
			var nested *pkgcontext.OperationError
			if failure.Operation == "dependency operation" || errors.As(failure.Err, &nested) {
				return collectDependencyFailureDiagnostics(failure.Err, failure.Err)
			}
		}
		return []diagnostic{dependencyOperationDiagnostic(failure)}
	case interface{ Unwrap() []error }:
		var results []diagnostic
		for _, cause := range failure.Unwrap() {
			results = append(results, collectDependencyFailureDiagnostics(cause, cause)...)
		}
		return results
	case interface{ Unwrap() error }:
		return collectDependencyFailureDiagnostics(failure.Unwrap(), details)
	default:
		message, action := dependencyFailureAdvice(details)
		result := diagnostic{Name: "dependencies", Status: checkFailed, Message: message, Action: action, Err: details}
		var pathErr *os.PathError
		if errors.As(details, &pathErr) {
			result.Message += ": " + filepath.Base(pathErr.Path)
			result.Path = pathErr.Path
		}
		return []diagnostic{result}
	}
}

func dependencyDestinationConflictDiagnostic(failure *pkgcontext.DependencyDestinationConflictError, details error) diagnostic {
	return diagnostic{
		Name:    "dependencies",
		Status:  checkFailed,
		Message: fmt.Sprintf("%s and %s both install to %q", failure.First, failure.Second, failure.Directory),
		Action:  "Update dependency declarations so this directory uses one repository and version.",
		Err:     details,
	}
}

func releaseAssetFailureDiagnostic(failure *download.ReleaseAssetNotFoundError, details error) diagnostic {
	meta := failure.Meta
	meta.Tag = failure.Tag
	meta.Branch = ""
	meta.Commit = ""
	message := fmt.Sprintf("%s: No release asset matches %q", meta, failure.Pattern)
	if len(failure.AvailableAssets) == 0 {
		message += "; this release has no assets"
	} else {
		message += "; available: " + strings.Join(failure.AvailableAssets, ", ")
	}
	return diagnostic{
		Name:    "dependencies",
		Status:  checkFailed,
		Message: message,
		Action:  "Choose a release with the required file, or fix the package's resource settings.",
		Err:     details,
	}
}

func dependencyRecoveryDiagnostics(err error) []diagnostic {
	switch failure := err.(type) {
	case *pkgcontext.OperationError:
		return []diagnostic{dependencyOperationDiagnostic(failure)}
	case interface{ Unwrap() []error }:
		var results []diagnostic
		for _, cause := range failure.Unwrap() {
			results = append(results, dependencyRecoveryDiagnostics(cause)...)
		}
		return results
	case interface{ Unwrap() error }:
		return dependencyRecoveryDiagnostics(failure.Unwrap())
	default:
		return nil
	}
}

func dependencyOperationDiagnostic(failure *pkgcontext.OperationError) diagnostic {
	result := diagnostic{Name: failure.Operation, Status: checkFailed, Err: failure}
	var pathErr *os.PathError
	if errors.As(failure.Err, &pathErr) {
		result.Path = pathErr.Path
	}
	switch failure.Operation {
	case "read manifest":
		result.Message = "Cannot read project configuration"
		if isConfigurationParseError(failure.Err) {
			result.Message = "Project configuration is invalid"
		}
		result.Action = "Check the contents and permissions of pawn.json or pawn.yaml, then retry."
	case "read lockfile":
		result.Message = "Cannot read pawn.lock"
		if isConfigurationParseError(failure.Err) {
			result.Message = "pawn.lock is invalid"
		}
		result.Action = "Check pawn.lock's contents and permissions. Run `sampctl check` for details."
	case "manifest":
		result.Message = "Cannot save project configuration"
		result.Action = "Check write access to pawn.json or pawn.yaml and the project directory."
	case "lockfile":
		result.Message = "Cannot update pawn.lock"
		result.Action = "Check write access to pawn.lock and the project directory, then run `sampctl ensure`."
	case "rollback":
		result.Message = "Could not restore all previous dependencies"
		result.Action = "Use --verbose to see the install and backup paths. Keep the backups until your dependencies are restored."
	case "cleanup":
		result.Message = "Cannot remove temporary dependency files"
		result.Action = "Use --verbose to see the temporary files. Keep the backups until the installation is verified."
	case "recovery":
		result.Message = "Dependency recovery failed"
		result.Action = "Run `sampctl check`. Keep the backups and recovery files until recovery finishes."
	case "project":
		result.Message = "Cannot start a dependency operation"
		result.Action = "Check project permissions. Wait for any other dependency command to finish, then retry."
	default:
		result.Message, result.Action = dependencyFailureAdvice(failure.Err)
	}
	return result
}

func dependencyFailureAdvice(err error) (string, string) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "Dependency command was cancelled", "Run the command again to retry."
	case errors.Is(err, transport.ErrRepositoryNotFound), errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed):
		return "Cannot access repository", "Check the repository name and permissions. If it needs authentication, check your Git credentials."
	case errors.Is(err, os.ErrPermission):
		return "Cannot access dependency files", "Check permissions on the project and sampctl cache directories."
	case errors.Is(err, os.ErrNotExist):
		return "Dependency files are missing", "Check dependency paths in pawn.json or pawn.yaml. Run `sampctl check` for details."
	case errors.Is(err, os.ErrInvalid):
		return "Dependency path is unsafe", "Use --verbose to see the path. Dependency files must stay inside their installation directories."
	case errors.Is(err, os.ErrExist):
		return "Files already exist at the install path", "Use --verbose to find the conflicting path. Move those files aside or choose another install directory, then retry."
	case isConfigurationParseError(err):
		return "Package configuration is invalid", "Fix pawn.json or pawn.yaml, then run `sampctl check`."
	}
	var remote *github.ErrorResponse
	if errors.As(err, &remote) && remote.Response != nil {
		switch remote.Response.StatusCode {
		case 401, 403, 404:
			return "Cannot access repository", "Check the repository name and permissions. If it needs authentication, check your Git credentials."
		case 429, 500, 502, 503, 504:
			return "Remote request failed", "Check your connection or wait for the service to recover, then retry."
		}
	}
	var network net.Error
	if errors.As(err, &network) {
		return "Network request failed", "Check your connection, then retry."
	}
	return "Dependency command failed", "Run the command with --verbose to see the error."
}

func isConfigurationParseError(err error) bool {
	var syntax *json.SyntaxError
	var jsonType *json.UnmarshalTypeError
	var yamlType *yaml.TypeError
	return errors.As(err, &syntax) || errors.As(err, &jsonType) || errors.As(err, &yamlType)
}

var diagnosticSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s@]+@`),
	regexp.MustCompile(`(?i)([?&](?:access_token|token|password|secret|api_key|key|credential|signature|sig|x-amz-signature|x-amz-credential|x-amz-security-token)=)[^&\s"'<>]+`),
	regexp.MustCompile(`(?im)((?:(?:proxy-)?authorization|x-api-key|api-key|x-auth-token|private-token|x-gitlab-token)\s*:\s*)[^\r\n]+`),
	regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|credential)["']?\s*[:=]\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|credential)["']?\s*[:=]\s*)'(?:\\.|[^'\\])*'`),
	regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|credential)["']?\s*[:=]\s*["']?)[^\s,;"'}]+`),
	regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)`),
}

func redactDiagnosticText(text string) string {
	for i, pattern := range diagnosticSecretPatterns {
		replacement := "${1}[redacted]"
		if i == 0 {
			replacement += "@"
		} else if i == len(diagnosticSecretPatterns)-1 {
			replacement = "[redacted]"
		}
		text = pattern.ReplaceAllString(text, replacement)
	}
	return text
}
