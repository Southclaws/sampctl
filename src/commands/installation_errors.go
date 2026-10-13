package commands

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/build/compiler"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
)

func runtimeInstallFailureDiagnostic(failure *runtimepkg.RuntimeInstallError, details error) diagnostic {
	message := "Could not install runtime " + failure.Runtime
	_, action := dependencyFailureAdvice(failure.Err)
	if failure.Committed {
		message = "Runtime " + failure.Runtime + " installed"
		if failure.RecoveryPending {
			message += ". Cleanup pending"
		}
	} else if failure.Restored {
		message += ". Previous runtime state restored"
	}
	if failure.RecoveryPending {
		action = "Run `sampctl check` and follow its recovery instructions. Keep the backups."
	} else if errors.Is(failure.Err, os.ErrExist) {
		action = "Move the conflicting or modified runtime file aside before retrying. Use --verbose for its path."
	}
	return diagnostic{Name: "runtime", Status: checkFailed, Message: message, Action: action, Err: details}
}

func compilerInstallFailureDiagnostic(failure *compiler.InstallationError, details error) diagnostic {
	message := "Could not prepare compiler " + filepath.Base(failure.Path)
	action := "Run `sampctl check` to inspect the compiler installation."
	if failure.Committed {
		message = "Compiler installation saved"
		if failure.RecoveryPending {
			message += ". Cleanup pending"
		}
	} else if failure.Restored {
		message += ". Previous compiler restored"
	}
	if failure.RecoveryPending {
		action = "Run `sampctl check` and follow its recovery instructions. Keep the backups."
	}
	return diagnostic{Name: "compiler", Status: checkFailed, Message: message, Action: action, Path: failure.Path, Err: details}
}

func compilerExecutionFailureDiagnostic(failure *compiler.ExecutionError, details error) diagnostic {
	result := diagnostic{Name: "compiler", Status: checkFailed, Path: failure.Path, Err: details}
	switch failure.Kind {
	case compiler.ExecutionMissing:
		result.Message = "Compiler executable is missing"
		result.Action = "Run `sampctl check` to inspect the compiler installation."
	case compiler.ExecutionStart:
		result.Message = "Compiler could not start"
		result.Action = "Check the compiler permissions and required libraries. Use --verbose for the error."
	case compiler.ExecutionCrash:
		result.Message = "Compiler exited unexpectedly"
		result.Action = "Run with --verbose to see the compiler output. Check its libraries and version."
	case compiler.ExecutionCancelled:
		result.Message = "Compilation cancelled"
	case compiler.ExecutionOutput:
		result.Message = "Compiler output could not be read or written"
		result.Action = "Check the output path and available disk space. Use --verbose for the error."
	default:
		result.Message = "Compiler command failed"
		result.Action = "Run with --verbose to see the compiler output."
	}
	return result
}
