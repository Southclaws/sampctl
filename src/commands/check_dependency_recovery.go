package commands

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func checkDependencyRecovery(ctx context.Context, root string, fix bool) ([]diagnostic, pkgcontext.DependencyRecoveryStatus, error) {
	state, err := pkgcontext.InspectDependencyRecovery(ctx, root)
	if ctx.Err() != nil {
		return nil, state, ctx.Err()
	}
	if !state.Pending && !state.Active && err == nil {
		return nil, state, nil
	}
	result := dependencyRecoveryDiagnostic(state, err)
	if !fix || !result.CanFix {
		return []diagnostic{result}, state, nil
	}
	result, err = repairDiagnostic(ctx, result, func(ctx context.Context) error {
		_, recoveryErr := pkgcontext.RecoverDependencyOperation(ctx, root)
		return recoveryErr
	}, func() diagnostic {
		var inspectErr error
		state, inspectErr = pkgcontext.InspectDependencyRecovery(ctx, root)
		return dependencyRecoveryDiagnostic(state, inspectErr)
	})
	return []diagnostic{result}, state, err
}

func dependencyRecoveryDiagnostic(state pkgcontext.DependencyRecoveryStatus, err error) diagnostic {
	result := diagnostic{Name: "dependency recovery", Status: checkFailed, Path: state.Path, Err: err}
	switch {
	case state.Active:
		result.Message = "A dependency operation is using this project"
		result.Action = "Wait for it to finish, then run `sampctl check` again."
	case err != nil:
		result.Message = "Dependency recovery files are invalid or inconsistent"
		result.Action = "Use --verbose to inspect the recovery files. Keep backups and recovery metadata until the project is restored."
	case !state.Pending:
		result.Status = checkPassed
		result.Message = "Dependency recovery is complete"
	case state.Committed:
		result.Message = "Project changes were saved; dependency cleanup is pending"
		result.Action = "Run `sampctl check --fix` or retry the dependency command to finish cleanup."
	case state.Restored:
		result.Message = "Previous dependency state was restored; cleanup is pending"
		result.Action = "Run `sampctl check --fix` or retry the dependency command to finish cleanup."
	default:
		result.Message = "A dependency operation was interrupted"
		result.Action = "Run `sampctl check --fix` or retry the dependency command to recover the previous project state."
	}
	result.CanFix = state.Pending && state.CanRecover && !state.Active && err == nil
	if state.Pending && !result.CanFix && err == nil && !state.Active {
		result.Action = "Use --verbose to inspect the recovery files. Keep backups and recovery metadata until the project is restored."
	}
	return result
}
