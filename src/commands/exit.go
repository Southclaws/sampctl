package commands

import "errors"

type commandExitError struct {
	code     int
	err      error
	reported bool
	message  string
}

func (e *commandExitError) Error() string {
	if e.err == nil || e.reported {
		return ""
	}
	if e.message != "" {
		return e.message
	}
	return redactDiagnosticText(e.err.Error())
}

func (e *commandExitError) Unwrap() error {
	return e.err
}

// ExitCode returns the process status for a command result.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *commandExitError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}
	return 1
}
