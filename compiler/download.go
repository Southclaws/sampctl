package compiler

import (
	"fmt"
	"os"
	"regexp"

	"github.com/Southclaws/sampctl/versioning"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

// Package represents a compiler package for a specific OS
type Package struct {
	Match  *regexp.Regexp       // the release asset name pattern
	Method download.ExtractFunc // the extraction method
	Binary string               // execution binary
	Paths  map[string]string    // map of files to their target locations
}

var (
	matchAssetMacOS = regexp.MustCompile(`pawnc-(.+)-(darwin|macos)\.zip`)
	matchAssetWin32 = regexp.MustCompile(`pawnc-(.+)-(windows)\.zip`)
	matchAssetLinux = regexp.MustCompile(`pawnc-(.+)-(linux)\.tar\.gz`)
)

var (
	pawnMacOS = Package{
		matchAssetMacOS,
		download.Unzip,
		"pawncc",
		map[string]string{
			"pawnc-(.+)/bin/pawncc":         "pawncc",
			"pawnc-(.+)/lib/libpawnc.dylib": "libpawnc.dylib",
		},
	}
	pawnLinux = Package{
		matchAssetLinux,
		download.Untar,
		"pawncc",
		map[string]string{
			"pawnc-(.+)/bin/pawncc":      "pawncc",
			"pawnc-(.+)/lib/libpawnc.so": "libpawnc.so",
		},
	}
	pawnWin32 = Package{
		matchAssetWin32,
		download.Unzip,
		"pawncc.exe",
		map[string]string{
			"pawnc-(.+)/bin/pawncc.exe": "pawncc.exe",
			"pawnc-(.+)/bin/pawnc.dll":  "pawnc.dll",
		},
	}
)

// FromCache attempts to get a compiler package from the cache, `hit` represents success
func FromCache(meta versioning.DependencyMeta, dir, platform, cacheDir string) (pkg *Package, hit bool, err error) {
	pkg = GetCompilerPackageInfo(platform)
	if pkg == nil {
		err = errors.Errorf("no compiler for platform '%s'", platform)
		return
	}

	filename := fmt.Sprintf("pawn-%s-%s", meta.Version, platform)

	print.Verb("Checking for cached package", filename, "in", cacheDir)

	hit, err = download.FromCache(cacheDir, filename, dir, pkg.Method, pkg.Paths)
	if !hit {
		return nil, false, nil
	}

	print.Verb("Using cached package", filename)

	return
}

// FromNet downloads a compiler package to the cache
func FromNet(meta versioning.DependencyMeta, dir, platform, cacheDir string) (pkg *Package, err error) {
	print.Info("Downloading compiler package", meta.Version)

	pkg = GetCompilerPackageInfo(platform)

	if !util.Exists(dir) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !util.Exists(cacheDir) {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	path, err := download.ReleaseAssetByPattern(meta, pkg.Match, "", fmt.Sprintf("pawn-%s-%s", meta.Version, platform), cacheDir)
	if err != nil {
		return
	}

	err = pkg.Method(path, dir, pkg.Paths)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unzip package %s", path)
	}

	return
}

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(version types.CompilerVersion, dir, platform, cacheDir string) (pkg *Package, err error) {
	meta := versioning.DependencyMeta{"Zeex", "pawn", "", string(version)}
	if meta.Version == "" {
		meta.Version = "3.10.5"
	}
	if meta.Version[0] != 'v' {
		meta.Version = "v" + meta.Version
	}

	pkg, hit, err := FromCache(meta, dir, platform, cacheDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	pkg, err = FromNet(meta, dir, platform, cacheDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// GetCompilerPackageInfo returns the URL for a specific compiler version
func GetCompilerPackageInfo(platform string) (pkg *Package) {
	switch platform {
	case "darwin":
		pkg = &pawnMacOS
	case "windows":
		pkg = &pawnWin32
	case "linux":
		pkg = &pawnLinux
	default:
		pkg = nil
	}
	return
}
