package compiler

import (
	"bytes"
	"html/template"
	"net/url"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/types"
)

// Package represents a compiler package for a specific OS
type Package struct {
	URL    string               // the URL template to get the package from
	Method download.ExtractFunc // the extraction method
	Binary string               // execution binary
	Paths  map[string]string    // map of files to their target locations
}

var (
	pawnMacOS = Package{
		"https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-darwin.zip",
		download.Unzip,
		"pawncc",
		map[string]string{
			"pawnc-{{.Version}}-darwin/bin/pawncc":         "pawncc",
			"pawnc-{{.Version}}-darwin/lib/libpawnc.dylib": "libpawnc.dylib",
		},
	}
	pawnLinux = Package{
		"https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-linux.tar.gz",
		download.Untar,
		"pawncc",
		map[string]string{
			"pawnc-{{.Version}}-linux/bin/pawncc":      "pawncc",
			"pawnc-{{.Version}}-linux/lib/libpawnc.so": "libpawnc.so",
		},
	}
	pawnWin32 = Package{
		"https://github.com/Zeex/pawn/releases/download/v{{.Version}}/pawnc-{{.Version}}-windows.zip",
		download.Unzip,
		"pawncc.exe",
		map[string]string{
			"pawnc-{{.Version}}-windows/bin/pawncc.exe": "pawncc.exe",
			"pawnc-{{.Version}}-windows/bin/pawnc.dll":  "pawnc.dll",
		},
	}
)

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(version types.CompilerVersion, dir string) (err error) {
	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return err
	}

	hit, err := FromCache(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	err = FromNet(cacheDir, version, dir)
	if err != nil {
		return errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// GetCompilerPackageInfo returns the URL for a specific compiler version
func GetCompilerPackageInfo(os string, version types.CompilerVersion) (pkg Package, filename string, err error) {
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
	err = tmpl.Execute(wr, struct{ Version types.CompilerVersion }{version})
	if err != nil {
		panic(err)
	}
	pkg.URL = wr.String()

	newPaths := make(map[string]string)
	for source, target := range pkg.Paths {
		sourceTmpl := template.Must(template.New("tmp2").Parse(source))
		sourceWriter := &bytes.Buffer{}
		err = sourceTmpl.Execute(sourceWriter, struct{ Version types.CompilerVersion }{version})
		if err != nil {
			panic(err)
		}

		targetTmpl := template.Must(template.New("tmp2").Parse(target))
		targetWriter := &bytes.Buffer{}
		err = targetTmpl.Execute(targetWriter, struct{ Version types.CompilerVersion }{version})
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
