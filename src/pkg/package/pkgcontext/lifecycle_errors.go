package pkgcontext

import "fmt"

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
}

func (e *OperationError) Error() string { return fmt.Sprintf("%s failed: %v", e.Operation, e.Err) }
func (e *OperationError) Unwrap() error { return e.Err }
