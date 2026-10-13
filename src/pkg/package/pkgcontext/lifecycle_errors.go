package pkgcontext

import (
	"errors"
	"fmt"
)

// DependencyOperationOutcome identifies which project changes survived an error.
type DependencyOperationOutcome string

const (
	OutcomeRolledBack         DependencyOperationOutcome = "rolled_back"
	OutcomeRollbackIncomplete DependencyOperationOutcome = "rollback_incomplete"
	OutcomeCommitted          DependencyOperationOutcome = "committed"
)

// DependencyError identifies a failed dependency operation while preserving its cause.
type DependencyError struct {
	Dependency string
	Operation  string
	Err        error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("%s dependency %s: %v", e.Operation, e.Dependency, e.Err)
}
func (e *DependencyError) Unwrap() error { return e.Err }

// OperationError identifies a failed project persistence or recovery step.
type OperationError struct {
	Operation string
	Err       error
	Outcome   DependencyOperationOutcome
}

func (e *OperationError) Error() string {
	var step *OperationError
	if e.Outcome != "" && errors.As(e.Err, &step) {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s failed: %v", e.Operation, e.Err)
}
func (e *OperationError) Unwrap() error { return e.Err }
