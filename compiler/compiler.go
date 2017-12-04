// Package compiler provides an API for acquiring the compiler binaries and compiling Pawn code
package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
)

// Version represents a compiler version number
type Version string

// Config represents a configuration for compiling a file
type Config struct {
	Name       string            `json:"name"`       // name of the configuration
	Args       []string          `json:"args"`       // list of arguments to pass to the compiler
	Constants  map[string]string `json:"constants"`  // set of constant definitions to pass to the compiler
	Version    Version           `json:"version"`    // compiler version to use for this build
	WorkingDir string            `json:"workingDir"` // working directory for the -D flag
	Input      string            `json:"input"`      // input .pwn file
	Output     string            `json:"output"`     // output .amx file
	Includes   []string          `json:"includes"`   // list of include files to include in compilation via -i flags
}

// GetDefaultConfig defines and returns a default compiler configuration
func GetDefaultConfig() Config {
	return Config{
		Args:    []string{"-d3", "-;+", "-(+", "-Z+"},
		Version: "3.10.4",
	}
}

// FromCache attempts to get a compiler package from the cache, `hit` represents success
func FromCache(cacheDir string, version Version, dir string) (hit bool, err error) {
	pkg, filename, err := GetCompilerPackageInfo(runtime.GOOS, version)
	if err != nil {
		return false, err
	}

	hit, err = download.FromCache(cacheDir, filename, dir, pkg.Method, pkg.Paths)
	if !hit {
		return false, nil
	}

	fmt.Printf("Using cached package %s\n", filename)

	return
}

// FromNet downloads a compiler package to the cache
func FromNet(cacheDir string, version Version, dir string) (err error) {
	fmt.Printf("Downloading compiler package %s\n", version)

	pkg, filename, err := GetCompilerPackageInfo(runtime.GOOS, version)
	if err != nil {
		return errors.Wrap(err, "package info mismatch")
	}

	if !util.Exists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !util.Exists(cacheDir) {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	path, err := download.FromNet(pkg.URL, cacheDir, filename)
	if err != nil {
		return errors.Wrap(err, "failed to download package")
	}

	err = pkg.Method(path, dir, pkg.Paths)
	if err != nil {
		return errors.Wrapf(err, "failed to unzip package %s", path)
	}

	return
}

// CompileSource compiles a given input script to the specified output path using compiler version
func CompileSource(cacheDir string, config Config) (err error) {
	fmt.Printf("Compiling source: '%s' with compiler %s...\n", config.Input, config.Version)

	if config.WorkingDir == "" {
		config.WorkingDir = filepath.Dir(config.Input)
	}

	cacheDir = util.FullPath(cacheDir)

	runtimeDir := filepath.Join(cacheDir, "pawn", string(config.Version))
	err = GetCompilerPackage(config.Version, runtimeDir)
	if err != nil {
		return
	}

	pkg, _, err := GetCompilerPackageInfo(runtime.GOOS, config.Version)
	if err != nil {
		return
	}

	args := []string{
		config.Input,
		"-D" + config.WorkingDir,
		"-o" + config.Output,
	}
	args = append(args, config.Args...)

	for _, inc := range config.Includes {
		args = append(args, "-i"+inc)
	}

	for name, value := range config.Constants {
		args = append(args, fmt.Sprintf("%s=%s", name, value))
	}

	binary := filepath.Join(runtimeDir, pkg.Binary)

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{fmt.Sprintf("LD_LIBRARY_PATH=%s", runtimeDir)}
	err = cmd.Run()
	if err != nil {
		return
	}

	return
}
