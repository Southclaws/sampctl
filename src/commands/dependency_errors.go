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
	writeCheckResult(&output, diagnostic{Status: checkFailed, Message: dependencyFailureHeading(operation, err)}, false, style)
	for _, result := range dependencyFailureDiagnostics(err) {
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
		return &commandExitError{code: 1, err: errors.Join(err, fmt.Errorf("write dependency diagnostics: %w", writeErr)), message: "Dependency operation failed; diagnostic output could not be written."}
	}
	return &commandExitError{code: 1, err: err, reported: true}
}

func dependencyFailureHeading(operation string, err error) string {
	if dependencyCleanupFailed(err) {
		return "Dependency operation completed; cleanup failed"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "Dependency operation cancelled"
	}
	switch operation {
	case "install":
		return "Dependency installation incomplete"
	case "uninstall":
		return "Dependency removal incomplete"
	default:
		return "Dependency setup incomplete"
	}
}

func dependencyCleanupFailed(err error) bool {
	switch failure := err.(type) {
	case *pkgcontext.DependencyError:
		return false
	case *pkgcontext.OperationError:
		return failure.Operation == "cleanup"
	case interface{ Unwrap() []error }:
		causes := failure.Unwrap()
		for _, cause := range causes {
			if !dependencyCleanupFailed(cause) {
				return false
			}
		}
		return len(causes) > 0
	case interface{ Unwrap() error }:
		return dependencyCleanupFailed(failure.Unwrap())
	default:
		return false
	}
}

func dependencyFailureDiagnostics(err error) []diagnostic {
	return collectDependencyFailureDiagnostics(err, err)
}

func collectDependencyFailureDiagnostics(err, details error) []diagnostic {
	switch failure := err.(type) {
	case *pkgcontext.DependencyError:
		message, action := dependencyFailureAdvice(failure.Err)
		if failure.Operation == "select" {
			message = "Dependency is not declared in this project"
			action = "Check the dependency name in pawn.json or pawn.yaml, then retry."
		}
		results := []diagnostic{{Name: "dependencies", Status: checkFailed, Message: failure.Dependency + ": " + message, Action: action, Err: details}}
		return append(results, dependencyRecoveryDiagnostics(failure.Err)...)
	case *pkgcontext.OperationError:
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
		result.Action = "Check pawn.json or pawn.yaml and its read permissions, then retry."
	case "read lockfile":
		result.Message = "Cannot read pawn.lock"
		if isConfigurationParseError(failure.Err) {
			result.Message = "pawn.lock is invalid"
		}
		result.Action = "Check pawn.lock and its read permissions. Run `sampctl check` for details."
	case "manifest":
		result.Message = "Cannot save project configuration"
		result.Action = "Check write access to pawn.json or pawn.yaml and the project directory."
	case "lockfile":
		result.Message = "Cannot update pawn.lock"
		result.Action = "Check write access to pawn.lock and the project directory, then run `sampctl ensure`."
	case "rollback":
		result.Message = "Previous project state was not fully restored"
		result.Action = "Run `sampctl check`. Review project files and installed dependencies before retrying."
	case "cleanup":
		result.Message = "Cannot remove temporary dependency files"
		result.Action = "Use --verbose to find leftover files. Keep any backups until you verify the project."
	case "project":
		result.Message = "Cannot start a dependency operation"
		result.Action = "Check project permissions and wait for other dependency commands to finish, then retry."
	default:
		result.Message, result.Action = dependencyFailureAdvice(failure.Err)
	}
	return result
}

func dependencyFailureAdvice(err error) (string, string) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "Dependency operation was cancelled", "Run `sampctl check` before retrying `sampctl ensure`."
	case errors.Is(err, transport.ErrRepositoryNotFound), errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed):
		return "Repository could not be accessed", "Check the repository name and your Git credentials."
	case errors.Is(err, os.ErrPermission):
		return "Dependency files could not be accessed", "Check permissions for the project directory and managed cache."
	case errors.Is(err, os.ErrNotExist):
		return "Required dependency files were not found", "Check dependency paths in pawn.json or pawn.yaml, then run `sampctl check`."
	case errors.Is(err, os.ErrInvalid):
		return "Dependency path is unsafe", "Use --verbose to inspect the path. Keep dependency files inside their managed directories."
	case isConfigurationParseError(err):
		return "Package configuration is invalid", "Correct pawn.json or pawn.yaml, then run `sampctl check`."
	}
	var remote *github.ErrorResponse
	if errors.As(err, &remote) && remote.Response != nil {
		switch remote.Response.StatusCode {
		case 401, 403, 404:
			return "Repository could not be accessed", "Check the repository name and your Git credentials."
		case 429, 500, 502, 503, 504:
			return "Remote service could not complete the request", "Check your connection and retry when the service is available."
		}
	}
	var network net.Error
	if errors.As(err, &network) {
		return "Network request failed", "Check your network connection and retry the dependency operation."
	}
	return "Dependency operation could not complete", "Check dependency declarations and run `sampctl check`."
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
