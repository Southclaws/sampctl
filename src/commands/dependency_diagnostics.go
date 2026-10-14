package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"go.yaml.in/yaml/v3"

	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
)

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
		var runtimeFailure *runtime.RuntimeInstallError
		if errors.As(failure.Err, &runtimeFailure) {
			return []diagnostic{runtimeInstallFailureDiagnostic(runtimeFailure, details)}
		}

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

		var missingResource *runtime.PluginResourceNotFoundError
		if errors.As(failure.Err, &missingResource) {
			result = pluginResourceFailureDiagnostic(missingResource, details)
		}

		results := []diagnostic{result}
		return append(results, dependencyRecoveryDiagnostics(failure.Err)...)
	case *download.ReleaseAssetNotFoundError:
		return []diagnostic{releaseAssetFailureDiagnostic(failure, details)}
	case *pkgcontext.DependencyDestinationConflictError:
		return []diagnostic{dependencyDestinationConflictDiagnostic(failure, details)}
	case *runtime.PluginResourceNotFoundError:
		return []diagnostic{pluginResourceFailureDiagnostic(failure, details)}
	case *runtime.RuntimeInstallError:
		return []diagnostic{runtimeInstallFailureDiagnostic(failure, details)}
	case *compiler.InstallationError:
		return []diagnostic{compilerInstallFailureDiagnostic(failure, details)}
	case *compiler.ExecutionError:
		return []diagnostic{compilerExecutionFailureDiagnostic(failure, details)}
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
		message += ". This release has no assets"
	} else {
		message += ". Available: " + strings.Join(failure.AvailableAssets, ", ")
	}

	return diagnostic{
		Name:    "dependencies",
		Status:  checkFailed,
		Message: message,
		Action:  "Choose a release with the required file, or fix the package's resource settings.",
		Err:     details,
	}
}

func pluginResourceFailureDiagnostic(failure *runtime.PluginResourceNotFoundError, details error) diagnostic {
	message := fmt.Sprintf("No plugin resource for %s and runtime %s", failure.Platform, failure.Version)
	if failure.Meta.Repo != "" {
		message = failure.Meta.String() + ": " + message
	}

	return diagnostic{
		Name:    "dependencies",
		Status:  checkFailed,
		Message: message,
		Action:  "Choose a package or release that supports this platform and runtime.",
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

	var rateLimit *github.RateLimitError
	if errors.As(err, &rateLimit) {
		action := "Wait for GitHub's rate limit to reset, then retry."
		if !rateLimit.Rate.Reset.IsZero() {
			action = "Retry after " + rateLimit.Rate.Reset.UTC().Format("2006-01-02 15:04:05 UTC") + "."
		}

		if rateLimit.Rate.Limit > 0 && rateLimit.Rate.Limit <= 60 {
			action += " Set SAMPCTL_GITHUB_TOKEN for a higher limit."
		}

		return "GitHub API rate limit reached", action
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
