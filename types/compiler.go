package types

import "fmt"

// BuildConfig represents a configuration for compiling a file
type BuildConfig struct {
	Name       string            `json:"name"`       // name of the configuration
	Version    CompilerVersion   `json:"version"`    // compiler version to use for this build
	WorkingDir string            `json:"workingDir"` // working directory for the -D flag
	Args       []string          `json:"args"`       // list of arguments to pass to the compiler
	Input      string            `json:"input"`      // input .pwn file
	Output     string            `json:"output"`     // output .amx file
	Includes   []string          `json:"includes"`   // list of include files to include in compilation via -i flags
	Constants  map[string]string `json:"constants"`  // set of constant definitions to pass to the compiler
}

// CompilerVersion represents a compiler version number
type CompilerVersion string

// GetBuildConfigDefault defines and returns a default compiler configuration
func GetBuildConfigDefault() *BuildConfig {
	return &BuildConfig{
		Args:    []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
		Version: "3.10.4",
	}
}

// ProblemSeverity represents the severity of a problem, warning error or fatal
type ProblemSeverity int8

const (
	// ProblemWarning is an issue that does not stop compilation but is still a concern
	ProblemWarning ProblemSeverity = iota
	// ProblemError is an issue that prevents AMX generation and may or may not stop compilation
	ProblemError ProblemSeverity = iota
	// ProblemFatal is an issue that stops compilation completely
	ProblemFatal ProblemSeverity = iota
)

func (ps ProblemSeverity) String() string {
	switch ps {
	case ProblemWarning:
		return "warning"
	case ProblemError:
		return "error"
	case ProblemFatal:
		return "fatal"
	}
	return "unknown"
}

// BuildProblem represents an issue with a line in a file with a severity level, these have a full
// file path, a line number, a severity level (warnings, errors and fatal errors) and a short
// description of the problem.
type BuildProblem struct {
	File        string
	Line        int
	Severity    ProblemSeverity
	Description string
}

// String creates a structured representation of a problem, for editor integration
func (bp BuildProblem) String() string {
	return fmt.Sprintf("%s:%d (%s) %s", bp.File, bp.Line, bp.Severity, bp.Description)
}

// BuildResult represents the final statistics (in bytes) of a successfully built .amx file.
type BuildResult struct {
	Header    int
	Code      int
	Data      int
	StackHeap int
	Estimate  int
	Total     int
}
