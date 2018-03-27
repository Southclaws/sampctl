package types

import "fmt"

// BuildConfig represents a configuration for compiling a file
type BuildConfig struct {
	Name       string            `json:"name"`                 // name of the configuration
	Version    CompilerVersion   `json:"version,omitempty"`    // compiler version to use for this build
	WorkingDir string            `json:"workingDir,omitempty"` // working directory for the -D flag
	Args       []string          `json:"args,omitempty"`       // list of arguments to pass to the compiler
	Input      string            `json:"input,omitempty"`      // input .pwn file
	Output     string            `json:"output,omitempty"`     // output .amx file
	Includes   []string          `json:"includes,omitempty"`   // list of include files to include in compilation via -i flags
	Constants  map[string]string `json:"constants,omitempty"`  // set of constant definitions to pass to the compiler
	Plugins    [][]string        `json:"plugins,omitempty"`    // set of commands to run before compilation
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

// BuildProblems is a slice of BuildProblem objects with additional methods
type BuildProblems []BuildProblem

// Warnings returns a slice of only warnings from a BuildProblems object
func (bps BuildProblems) Warnings() (warnings []BuildProblem) {
	for _, b := range bps {
		if b.Severity == ProblemWarning {
			warnings = append(warnings, b)
		}
	}
	return
}

// Errors returns a slice of only errors from a BuildProblems object
func (bps BuildProblems) Errors() (warnings []BuildProblem) {
	for _, b := range bps {
		if b.Severity == ProblemError {
			warnings = append(warnings, b)
		}
	}
	return
}

// Fatal returns true if the build problems contain any fatal problems
func (bps BuildProblems) Fatal() (fatal bool) {
	for _, b := range bps {
		if b.Severity == ProblemFatal {
			return true
		}
	}
	return false
}

// IsValid returns true if the BuildProblems only contains warnings, if there are errors it's false
func (bps BuildProblems) IsValid() bool {
	return len(bps.Errors()) == 0
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
