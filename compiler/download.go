package compiler

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Package represents a compiler package for a specific OS
type Package struct {
	Match  string                   // the release asset name pattern
	Method download.ExtractFuncName // the extraction method
	Binary string                   // execution binary
	Paths  map[string]string        // map of files to their target locations
}

// Packages is a hard coded map of platforms to Package objects
// todo: store this remotely and load on startup
var Packages = map[string]*Package{
	"darwin": &Package{
		`pawnc-(.+)-(darwin|macos)\.zip`,
		"zip",
		"pawncc",
		map[string]string{
			"pawnc-(.+)/bin/pawncc":         "pawncc",
			"pawnc-(.+)/lib/libpawnc.dylib": "libpawnc.dylib",
		},
	},
	"linux": &Package{
		`pawnc-(.+)-(linux)\.tar\.gz`,
		"tgz",
		"pawncc",
		map[string]string{
			"pawnc-(.+)/bin/pawncc":      "pawncc",
			"pawnc-(.+)/lib/libpawnc.so": "libpawnc.so",
		},
	},
	"windows": &Package{
		`pawnc-(.+)-(windows)\.zip`,
		"zip",
		"pawncc.exe",
		map[string]string{
			"pawnc-(.+)/bin/pawncc.exe": "pawncc.exe",
			"pawnc-(.+)/bin/pawnc.dll":  "pawnc.dll",
		},
	},
}

// FromCache attempts to get a compiler package from the cache, `hit` represents success
func FromCache(meta versioning.DependencyMeta, dir, platform, cacheDir string) (pkg *Package, hit bool, err error) {
	pkg = GetCompilerPackageInfo(platform)
	if pkg == nil {
		err = errors.Errorf("no compiler for platform '%s'", platform)
		return
	}

	filename := fmt.Sprintf("pawn-%s-%s", meta.Tag, platform)

	print.Verb("Checking for cached package", filename, "in", cacheDir)

	hit, err = download.FromCache(cacheDir, filename, dir, download.ExtractFuncFromName(pkg.Method), pkg.Paths)
	if !hit {
		return nil, false, nil
	}

	print.Verb("Using cached package", filename)

	return
}

// FromNet downloads a compiler package to the cache
func FromNet(ctx context.Context, gh *github.Client, meta versioning.DependencyMeta, dir, platform, cacheDir string) (pkg *Package, err error) {
	print.Info("Downloading compiler package", meta.Tag)

	pkg = GetCompilerPackageInfo(platform)

	if !util.Exists(dir) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	if !util.Exists(cacheDir) {
		err = os.MkdirAll(cacheDir, 0700)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create cache %s", cacheDir)
		}
	}

	path, err := download.ReleaseAssetByPattern(ctx, gh, meta, regexp.MustCompile(pkg.Match), "", fmt.Sprintf("pawn-%s-%s", meta.Tag, platform), cacheDir)
	if err != nil {
		return
	}

	method := download.ExtractFuncFromName(pkg.Method)
	if method == nil {
		return nil, errors.Errorf("invalid extract type: %s", pkg.Method)
	}

	err = method(path, dir, pkg.Paths)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unzip package %s", path)
	}

	return
}

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(ctx context.Context, gh *github.Client, version types.CompilerVersion, dir, platform, cacheDir string) (pkg *Package, err error) {
	meta := versioning.DependencyMeta{
		Site: "github.com",
		User: "pawn-lang",
		Repo: "compiler",
		Tag:  string(version),
	}

	if meta.Tag == "" {
		meta.Tag = "3.10.4"
	}
	if meta.Tag[0] != 'v' {
		meta.Tag = "v" + meta.Tag
	}

	pkg, hit, err := FromCache(meta, dir, platform, cacheDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get package %s from cache", version)
	}
	if hit {
		return
	}

	pkg, err = FromNet(ctx, gh, meta, dir, platform, cacheDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get package %s from net", version)
	}

	return
}

// GetCompilerPackageInfo returns the URL for a specific compiler version
func GetCompilerPackageInfo(platform string) (pkg *Package) {
	pkg, ok := Packages[platform]
	if !ok {
		pkg = nil
	}
	return
}
