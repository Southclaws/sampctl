// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"fmt"
	"io/ioutil"
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

	includePaths := make(map[string]struct{})
	includeFiles := make(map[string]string)
	includeErrors := []string{}

	var fullPath string
	for _, inc := range config.Includes {
		if filepath.IsAbs(inc) {
			fullPath = inc
		} else {
			fullPath = filepath.Join(execDir, inc)
		}

		if _, found := includePaths[fullPath]; found {
			fmt.Println("- ignoring duplicate", fullPath)
			continue
		}
		includePaths[fullPath] = struct{}{}

		fmt.Println("- using include path", fullPath)
		args = append(args, "-i"+fullPath)

		contents, err := ioutil.ReadDir(fullPath)
		if err != nil {
			return errors.Wrapf(err, "failed to list dependency include path:", inc)
		}

		for _, dependencyFile := range contents {
			fileName := dependencyFile.Name()
			fileExt := filepath.Ext(fileName)
			if fileExt == ".inc" {
				if location, exists := includeFiles[fileName]; exists {
					if location != fullPath {
						includeErrors = append(includeErrors, fmt.Sprintf("Duplicate '%s' found in both\n'%s'\n'%s'\n", fileName, location, fullPath))
					}
				} else {
					includeFiles[fileName] = fullPath
				}
			}
		}
	}

	if len(includeErrors) > 0 {
		fmt.Println("Dependency include path errors found:")
		for _, errorString := range includeErrors {
			fmt.Println(errorString)
		}
		return errors.New("could not compile due to conflicting filenames located in different include paths")
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
		return errors.Wrap(err, "compilation failed")
	}

	return
}
