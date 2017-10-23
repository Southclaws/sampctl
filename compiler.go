package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// CompilerPackage represents a compiler package for a specific OS
type CompilerPackage struct {
	URL    string            // the URL template to get the package from
	Method ExtractFunc       // the extraction method
	Binary string            // execution binary
	Paths  map[string]string // map of files to their target locations
}

var (
	pawnMacOS = CompilerPackage{
		"https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-darwin.zip",
		Unzip,
		"pawncc",
		map[string]string{
			"pawnc-{{.Version}}-darwin/bin/pawncc":         "pawncc",
			"pawnc-{{.Version}}-darwin/lib/libpawnc.dylib": "libpawnc.dylib",
		},
	}
	pawnLinux = CompilerPackage{
		"https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-linux.tar.gz",
		Untar,
		"pawncc",
		map[string]string{
			"pawnc-{{.Version}}-linux/bin/pawncc":      "pawncc",
			"pawnc-{{.Version}}-linux/lib/libpawnc.so": "libpawnc.so",
		},
	}
	pawnWin32 = CompilerPackage{
		"https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-windows.zip",
		Unzip,
		"pawncc.exe",
		map[string]string{
			"pawnc-{{.Version}}-windows/bin/pawncc.exe": "pawncc.exe",
			"pawnc-{{.Version}}-windows/bin/pawnc.dll":  "pawnc.dll",
		},
	}
)

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(version, dir string) (err error) {
	cacheDir, err := GetCacheDir()
	if err != nil {
		return err
	}

	hit, err := CompilerFromCache(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = CompilerFromNet(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// GetCompilerPackageInfo returns the URL for a specific compiler version
func GetCompilerPackageInfo(os, version string) (pkg CompilerPackage, filename string, err error) {
	if os == "windows" {
		pkg = pawnWin32
	} else if os == "linux" {
		pkg = pawnLinux
	} else if os == "darwin" {
		pkg = pawnMacOS
	} else {
		err = errors.Errorf("unsupported OS %s", runtime.GOOS)
		return
	}

	tmpl := template.Must(template.New("tmp1").Parse(pkg.URL))
	wr := &bytes.Buffer{}
	err = tmpl.Execute(wr, struct{ Version string }{version})
	if err != nil {
		panic(err)
	}
	pkg.URL = wr.String()

	newPaths := make(map[string]string)
	for source, target := range pkg.Paths {
		sourceTmpl := template.Must(template.New("tmp2").Parse(source))
		sourceWriter := &bytes.Buffer{}
		err = sourceTmpl.Execute(sourceWriter, struct{ Version string }{version})
		if err != nil {
			panic(err)
		}

		targetTmpl := template.Must(template.New("tmp2").Parse(target))
		targetWriter := &bytes.Buffer{}
		err = targetTmpl.Execute(targetWriter, struct{ Version string }{version})
		if err != nil {
			panic(err)
		}

		newPaths[sourceWriter.String()] = targetWriter.String()
	}
	pkg.Paths = newPaths

	u, err := url.Parse(pkg.URL)
	if err != nil {
		return
	}
	filename = filepath.Base(u.Path)

	return
}

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
	fmt.Println(pkg.Paths)

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

	dir := filepath.Join(cacheDir, "pawn", version)
	err = GetCompilerPackage(version, dir)
	if err != nil {
		return
	}

	pkg, _, err := GetCompilerPackageInfo(runtime.GOOS, version)
	if err != nil {
		return
	}

	binary := filepath.Join(cacheDir, "pawn", version, pkg.Binary)

	cmd := exec.Command(binary, input, "-;+", "-(+", "-d3", "-Z+", "-o"+output)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return
	}

	return
}
