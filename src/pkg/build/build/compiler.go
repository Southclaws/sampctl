package build

import (
	"fmt"
	"strconv"
)

// CompilerOptions represents human-readable compiler flags
type CompilerOptions struct {
	DebugLevel             *int    `json:"debug_level,omitempty" yaml:"debug_level,omitempty"`                           // -d<level> | 0=none, 1=minimal, 2=full, 3=extended | default=3
	RequireSemicolons      *bool   `json:"require_semicolons,omitempty" yaml:"require_semicolons,omitempty"`             // -;+ <enforce> | -;- <relax> | default=true
	RequireParentheses     *bool   `json:"require_parentheses,omitempty" yaml:"require_parentheses,omitempty"`           // -(+ <enforce> | -(- <relax> | default=true
	RequireEscapeSequences *bool   `json:"require_escape_sequences,omitempty" yaml:"require_escape_sequences,omitempty"` // -\\+ <enforce> | -\\- <relax> | default=true
	CompatibilityMode      *bool   `json:"compatibility_mode,omitempty" yaml:"compatibility_mode,omitempty"`             // -Z+ <enable> | -Z- <disable> | default=true
	OptimizationLevel      *int    `json:"optimization_level,omitempty" yaml:"optimization_level,omitempty"`             // -O<level> | 0=none, 1=basic, 2=full | default=none
	ShowListing            *bool   `json:"show_listing,omitempty" yaml:"show_listing,omitempty"`                         // -l <enable> | -l- <disable> | default=false
	ShowAnnotatedAssembly  *bool   `json:"show_annotated_assembly,omitempty" yaml:"show_annotated_assembly,omitempty"`   // -a <enable> | -a- <disable> | default=false
	ShowErrorFile          *string `json:"show_error_file,omitempty" yaml:"show_error_file,omitempty"`                   // -e<filename> | default=""
	ShowWarnings           *bool   `json:"show_warnings,omitempty" yaml:"show_warnings,omitempty"`                       // -w+ to enable, -w- to disable | default=enabled
	CompactEncoding        *bool   `json:"compact_encoding,omitempty" yaml:"compact_encoding,omitempty"`                 // -C+ to enable, -C- to disable | default=disabled
	TabSize                *int    `json:"tab_size,omitempty" yaml:"tab_size,omitempty"`                                 // -t<spaces> | default=4
}

// ToArgs converts CompilerOptions to a slice of command-line arguments
func (opts *CompilerOptions) ToArgs() []string {
	if opts == nil {
		return nil
	}

	var args []string

	if opts.DebugLevel != nil {
		args = append(args, "-d"+strconv.Itoa(*opts.DebugLevel))
	}

	if opts.RequireSemicolons != nil {
		if *opts.RequireSemicolons {
			args = append(args, "-;+")
		} else {
			args = append(args, "-;-")
		}
	}

	if opts.RequireParentheses != nil {
		if *opts.RequireParentheses {
			args = append(args, "-(+")
		} else {
			args = append(args, "-(-")
		}
	}

	if opts.RequireEscapeSequences != nil {
		if *opts.RequireEscapeSequences {
			args = append(args, "-\\+")
		} else {
			args = append(args, "-\\-")
		}
	}

	if opts.CompatibilityMode != nil {
		if *opts.CompatibilityMode {
			args = append(args, "-Z+")
		} else {
			args = append(args, "-Z-")
		}
	}

	if opts.OptimizationLevel != nil {
		args = append(args, "-O"+strconv.Itoa(*opts.OptimizationLevel))
	}

	if opts.ShowListing != nil && *opts.ShowListing {
		args = append(args, "-l")
	}

	if opts.ShowAnnotatedAssembly != nil && *opts.ShowAnnotatedAssembly {
		args = append(args, "-a")
	}

	if opts.ShowErrorFile != nil && *opts.ShowErrorFile != "" {
		args = append(args, "-e"+*opts.ShowErrorFile)
	}

	if opts.ShowWarnings != nil {
		if *opts.ShowWarnings {
			args = append(args, "-w+")
		} else {
			args = append(args, "-w-")
		}
	}

	if opts.CompactEncoding != nil {
		if *opts.CompactEncoding {
			args = append(args, "-C+")
		} else {
			args = append(args, "-C-")
		}
	}

	if opts.TabSize != nil {
		args = append(args, "-t"+strconv.Itoa(*opts.TabSize))
	}

	return args
}

// Config represents a configuration for compiling a file
type Config struct {
	Name              string            `json:"name" yaml:"name"`                                 // name of the configuration
	Version           CompilerVersion   `json:"version,omitempty" yaml:"version,omitempty"`       // compiler version to use for this build
	WorkingDir        string            `json:"workingDir,omitempty" yaml:"workingDir,omitempty"` // working directory for the -D flag
	Args              []string          `json:"args,omitempty" yaml:"args,omitempty"`             // list of arguments to pass to the compiler (deprecated)
	Options           *CompilerOptions  `json:"options,omitempty" yaml:"options,omitempty"`       // human-readable compiler options (use this in future over args)
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
	Preset  string `json:"preset,omitempty" yaml:"preset,omitempty"`   // Predefined compiler preset (pawn-lang, openmp, etc.)
}

// CompilerPreset represents a predefined compiler configuration
type CompilerPreset struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Site        string `json:"site"`
	User        string `json:"user"`
	Repo        string `json:"repo"`
	Version     string `json:"version"`
}

// GetPredefinedCompilers returns a map of predefined compiler configurations
func GetPredefinedCompilers() map[string]CompilerPreset {
	return map[string]CompilerPreset{
		"openmp": {
			Name:        "openmp",
			Description: "Open.MP modified Pawn compiler with improvements",
			Site:        "github.com",
			User:        "openmultiplayer",
			Repo:        "compiler",
			Version:     "3.10.11",
		},
		"samp": {
			Name:        "samp",
			Description: "Default compiler for SA-MP (Community Compiler)",
			Site:        "github.com",
			User:        "pawn-lang",
			Repo:        "compiler",
			Version:     "3.10.10",
		},
	}
}

// Default defines and returns a default compiler configuration
func Default() *Config {
	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }

	return &Config{
		Options: &CompilerOptions{
			DebugLevel:             intPtr(3),
			RequireSemicolons:      boolPtr(true),
			RequireParentheses:     boolPtr(true),
			RequireEscapeSequences: boolPtr(true),
			CompatibilityMode:      boolPtr(true),
		},
		Compiler: CompilerConfig{
			Preset: "samp",
		},
	}
}

// ResolveCompilerConfig resolves the final compiler configuration by applying presets
func (cc *CompilerConfig) ResolveCompilerConfig() CompilerConfig {
	resolved := *cc

	// If a preset is specified, apply it first
	if cc.Preset != "" {
		presets := GetPredefinedCompilers()
		if preset, exists := presets[cc.Preset]; exists {
			// Apply preset values only if they're not already set
			if resolved.Site == "" {
				resolved.Site = preset.Site
			}
			if resolved.User == "" {
				resolved.User = preset.User
			}
			if resolved.Repo == "" {
				resolved.Repo = preset.Repo
			}
			if resolved.Version == "" {
				resolved.Version = preset.Version
			}
		}
	}

	// Apply default values if still empty
	if resolved.Site == "" {
		resolved.Site = "github.com"
	}
	if resolved.User == "" {
		resolved.User = "pawn-lang"
	}
	if resolved.Repo == "" {
		resolved.Repo = "compiler"
	}
	if resolved.Version == "" {
		resolved.Version = "3.10.10"
	}

	return resolved
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
