// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/util"
)

// Version represents a compiler version number
type Version string

// Config represents a configuration for compiling a file
type Config struct {
	Name       string            `json:"name"`       // name of the configuration
	Version    Version           `json:"version"`    // compiler version to use for this build
	WorkingDir string            `json:"workingDir"` // working directory for the -D flag
	Args       []string          `json:"args"`       // list of arguments to pass to the compiler
	Input      string            `json:"input"`      // input .pwn file
	Output     string            `json:"output"`     // output .amx file
	Includes   []string          `json:"includes"`   // list of include files to include in compilation via -i flags
	Constants  map[string]string `json:"constants"`  // set of constant definitions to pass to the compiler
}

// GetDefaultConfig defines and returns a default compiler configuration
func GetDefaultConfig() Config {
	return Config{
		Args:    []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
		Version: "3.10.4",
	}
}

// CompileSource compiles a given input script to the specified output path using compiler version
func CompileSource(execDir string, cacheDir string, config Config) (err error) {
	fmt.Printf("Compiling source: '%s' with compiler %s...\n", config.Input, config.Version)

	if config.WorkingDir == "" {
		config.WorkingDir = filepath.Dir(config.Input)
	}

	cacheDir = util.FullPath(cacheDir)

	runtimeDir := filepath.Join(cacheDir, "pawn", string(config.Version))
	err = GetCompilerPackage(config.Version, runtimeDir)
	if err != nil {
		return errors.Wrap(err, "failed to get compiler package")
	}

	pkg, _, err := GetCompilerPackageInfo(runtime.GOOS, config.Version)
	if err != nil {
		return errors.Wrap(err, "failed to get compiler package info for runtime")
	}

	args := []string{
		config.Input,
		"-D" + config.WorkingDir,
		"-o" + config.Output,
	}
	args = append(args, config.Args...)

	var fullPath string
	for _, inc := range config.Includes {
		if filepath.IsAbs(inc) {
			fullPath = inc
		} else {
			fullPath = filepath.Join(execDir, inc)
		}

		fmt.Println("- using include path", fullPath)
		args = append(args, "-i"+fullPath)
	}

	for name, value := range config.Constants {
		args = append(args, fmt.Sprintf("%s=%s", name, value))
	}

	binary := filepath.Join(runtimeDir, pkg.Binary)

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		fmt.Sprintf("LD_LIBRARY_PATH=%s", runtimeDir),
		fmt.Sprintf("DYLD_LIBRARY_PATH=%s", runtimeDir),
	}
	fmt.Println(cmd.Args)
	err = cmd.Run()
	if err != nil {
		// todo: make a config flag to ignore this message
		fmt.Println("** if you're on a 64 bit system this may be because the system is not set up to execute 32 bit binaries")
		fmt.Println("** please enable this by allowing i386 packages and/or installing g++-multilib")
		return errors.Wrap(err, "failed to execute compiler")
	}

	return
}
