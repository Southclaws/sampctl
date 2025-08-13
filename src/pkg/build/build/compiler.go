package build

import (
	"fmt"
)

// Config represents a configuration for compiling a file
type Config struct {
	Name              string            `json:"name" yaml:"name"`                                 // name of the configuration
	Version           CompilerVersion   `json:"version,omitempty" yaml:"version,omitempty"`       // compiler version to use for this build
	WorkingDir        string            `json:"workingDir,omitempty" yaml:"workingDir,omitempty"` // working directory for the -D flag
	Args              []string          `json:"args,omitempty" yaml:"args,omitempty"`             // list of arguments to pass to the compiler
	Input             string            `json:"input,omitempty" yaml:"input,omitempty"`           // input .pwn file
	Output            string            `json:"output,omitempty" yaml:"output,omitempty"`         // output .amx file
	Includes          []string          `json:"includes,omitempty" yaml:"includes,omitempty"`     // list of include files to pass to compiler via -i flags
	Constants         map[string]string `json:"constants,omitempty" yaml:"constants,omitempty"`   // set of constant definitions to pass to the compiler
	Plugins           [][]string        `json:"plugins,omitempty" yaml:"plugins,omitempty"`       // set of commands to run before compilation
	Compiler          CompilerConfig    `json:"compiler,omitempty" yaml:"compiler,omitempty"`     // a set of configurations for using a compiler
	PreBuildCommands  [][]string        `json:"prebuild,omitempty" yaml:"prebuild,omitempty"`     // allows the execution of commands before a build is ran
	PostBuildCommands [][]string        `json:"postbuild,omitempty" yaml:"postbuild,omitempty"`   // allows the execution of commands after a build is ran
}

// CompilerVersion represents a compiler version number
type CompilerVersion string

// CompilerConfig represents a configuration for a compiler repository
type CompilerConfig struct {
	Site    string `json:"site,omitempty" yaml:"site,omitempty"`       // The site hosting the repo
	User    string `json:"user,omitempty" yaml:"user,omitempty"`       // Name of the github user
	Repo    string `json:"repo,omitempty" yaml:"repo,omitempty"`       // Name of the github repository
	Version string `json:"version,omitempty" yaml:"version,omitempty"` // The version of the compiler to use
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`       // The path to the compiler (overrides the above)
}

// Default defines and returns a default compiler configuration
func Default() *Config {
	return &Config{
		Args: []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
		Compiler: CompilerConfig{
			Site:    "github.com",
			User:    "pawn-lang",
			Repo:    "compiler",
			Version: "3.10.10",
			Path:    "",
		},
	}
}

// Result represents the final statistics (in bytes) of a successfully built .amx file.
type Result struct {
	Header    int
	Code      int
	Data      int
	StackHeap int
	Estimate  int
	Total     int
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

// Problem represents an issue with a line in a file with a severity level, these have a full
// file path, a line number, a severity level (warnings, errors and fatal errors) and a short
// description of the problem.
type Problem struct {
	File        string
	Line        int
	Severity    ProblemSeverity
	Description string
}

// String creates a structured representation of a problem, for editor integration
func (bp Problem) String() string {
	return fmt.Sprintf("%s:%d (%s) %s", bp.File, bp.Line, bp.Severity, bp.Description)
}

// Problems is a slice of Problem objects with additional methods
type Problems []Problem

// Warnings returns a slice of only warnings from a Problems object
func (bps Problems) Warnings() (warnings []Problem) {
	for _, b := range bps {
		if b.Severity == ProblemWarning {
			warnings = append(warnings, b)
		}
	}
	return
}

// Errors returns a slice of only errors from a Problems object
func (bps Problems) Errors() (warnings []Problem) {
	for _, b := range bps {
		if b.Severity == ProblemError {
			warnings = append(warnings, b)
		}
	}
	return
}

// Fatal returns true if the build problems contain any fatal problems
func (bps Problems) Fatal() (fatal bool) {
	for _, b := range bps {
		if b.Severity == ProblemFatal {
			return true
		}
	}
	return false
}

// IsValid returns true if the Problems only contains warnings, if there are errors it's false
func (bps Problems) IsValid() bool {
	return len(bps.Errors()) == 0
}
