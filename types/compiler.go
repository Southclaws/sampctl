package types

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
