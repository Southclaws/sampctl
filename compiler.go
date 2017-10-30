package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// CompilerFromCache attempts to get a compiler package from the cache, `hit` represents success
func CompilerFromCache(cacheDir, version, dir string) (hit bool, err error) {
	fmt.Printf("Using cached package for %s\n", version)

	pkg, filename, err := GetCompilerPackageInfo(runtime.GOOS, version)
	if err != nil {
		return false, err
	}

	hit, err = FromCache(cacheDir, filename, dir, pkg.Method, pkg.Paths)
	if !hit {
		return false, nil
	}

	return
}

// CompilerFromNet downloads a compiler package to the cache
func CompilerFromNet(cacheDir, version, dir string) (err error) {
	fmt.Printf("Downloading compiler package %s\n", version)

	pkg, filename, err := GetCompilerPackageInfo(runtime.GOOS, version)
	if err != nil {
		return errors.Wrap(err, "package info mismatch")
	}

	if !exists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !exists(cacheDir) {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	path, err := FromNet(pkg.URL, cacheDir, filename)
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
func CompileSource(input, output, cacheDir, version string) (err error) {
	fmt.Printf("Compiling source: '%s'...\n", input)

	cacheDir = fullPath(cacheDir)

	dir := filepath.Join(cacheDir, "pawn", version)
	err = GetCompilerPackage(version, dir)
	if err != nil {
		return
	}

	pkg, _, err := GetCompilerPackageInfo(runtime.GOOS, version)
	if err != nil {
		return
	}

	binary := filepath.Join(dir, pkg.Binary)

	cmd := exec.Command(binary, input, "-;+", "-(+", "-d3", "-Z+", "-o"+output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{fmt.Sprintf("LD_LIBRARY_PATH=%s", dir)}
	err = cmd.Run()
	if err != nil {
		return
	}

	return
}
