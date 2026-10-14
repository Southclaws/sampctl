package commands

import (
	"context"
	"fmt"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
)

func checkRuntime(ctx context.Context, pkg pawnpackage.Package, opts checkOptions) ([]diagnostic, error) {
	root := pkg.RuntimeWorkingDir()
	state, inspectErr := runtimepkg.InspectRuntimeRecovery(ctx, root)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var results []diagnostic
	if state.Pending || state.Active || inspectErr != nil {
		result := runtimeRecoveryDiagnostic(state, inspectErr)
		if opts.Fix && result.CanFix {
			var err error
			result, err = repairDiagnostic(ctx, result, func(ctx context.Context) error {
				return runtimepkg.RecoverRuntime(ctx, root)
			}, func() diagnostic {
				var inspectErr error
				state, inspectErr = runtimepkg.InspectRuntimeRecovery(ctx, root)
				return runtimeRecoveryDiagnostic(state, inspectErr)
			})
			if err != nil {
				return []diagnostic{result}, err
			}
		}

		results = append(results, result)
		if state.Pending || state.Active || inspectErr != nil {
			return results, ctx.Err()
		}
	}

	result := diagnostic{Name: "runtime", Path: root}
	info, err := runtimepkg.GetRuntimeManifestInfo(root)
	if err != nil {
		result.Status, result.Err = checkFailed, err
		result.Message = "Cannot read installed runtime ownership"
		result.Action = "Run `sampctl check --verbose`. Keep the runtime files and ownership record for inspection."
	} else if info == nil {
		result.Status, result.Message = checkSkipped, "No managed runtime installation to check"
	} else if err := runtimepkg.VerifyInstalledRuntime(root); err != nil {
		result.Status, result.Err = checkFailed, err
		result.Message = "Managed runtime files are missing or changed"
		result.Action = "Run `sampctl check --verbose` to inspect the files before retrying `sampctl ensure`."
	} else {
		cfg, err := pkg.GetRuntimeConfig("")
		if err != nil {
			result.Status, result.Err = checkFailed, err
			result.Message = "Cannot read runtime configuration"
		} else if info.Version != cfg.Version || info.Platform != opts.Platform {
			result.Status = checkWarning
			result.Message = fmt.Sprintf("Installed runtime %s for %s differs from the selected runtime", info.Version, info.Platform)
			result.Action = "Run `sampctl ensure` to install the selected runtime."
		} else {
			result.Status, result.Message = checkPassed, "Managed runtime files are valid"
		}
	}

	return append(results, result), ctx.Err()
}

func runtimeRecoveryDiagnostic(state runtimepkg.RuntimeRecoveryStatus, err error) diagnostic {
	result := diagnostic{Name: "runtime recovery", Status: checkFailed, Path: state.Path, Err: err}
	switch {
	case state.Active:
		result.Message = "A runtime update is running"
		result.Action = "Wait for it to finish, then retry."
	case err != nil:
		result.Message = "Runtime recovery files are invalid or changed"
		result.Action = "Run `sampctl check --verbose`. Keep the recovery record and backups for inspection."
	case !state.Pending:
		result.Status, result.Message = checkPassed, "Runtime recovery is complete"
	case state.Committed:
		result.Message = "Runtime update saved. Cleanup pending"
	case state.Restored:
		result.Message = "Previous runtime state restored. Cleanup pending"
	default:
		result.Message = "A runtime update was interrupted"
	}

	result.CanFix = state.Pending && state.CanRecover && !state.Active && err == nil
	if result.CanFix {
		result.Action = "Run `sampctl check --fix` or retry `sampctl ensure` to finish recovery."
	}

	return result
}
