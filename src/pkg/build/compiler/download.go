package compiler

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// FromCache attempts to get a compiler package from the cache, `hit` represents success
func FromCache(
	meta versioning.DependencyMeta,
	dir string,
	platform string,
	cacheDir string,
) (compiler download.Compiler, hit bool, err error) {
	compiler, err = GetCompilerPackageInfo(cacheDir, platform)
	if err != nil {
		return
	}

	filename := GetCompilerFilename(meta.Tag, platform, compiler.Method)

	print.Verb("Checking for cached package", filename, "in", cacheDir)

	hit, err = download.FromCache(
		cacheDir,
		filename,
		dir,
		download.ExtractFuncFromName(compiler.Method),
		compiler.Paths,
		platform)
	if !hit {
		return
	}

	print.Verb("Using cached package", filename)

	return
}

// FromNet downloads a compiler package to the cache
func FromNet(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	dir string,
	platform string,
	cacheDir string,
) (compiler download.Compiler, err error) {
	print.Info("Downloading compiler package", meta.Tag)

	compiler, err = GetCompilerPackageInfo(cacheDir, platform)
	if err != nil {
		return
	}

	if !util.Exists(dir) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			err = errors.Wrapf(err, "failed to create dir %s", dir)
			return
		}
	}

	path, _, err := download.ReleaseAssetByPattern(
		ctx,
		gh,
		meta,
		regexp.MustCompile(compiler.Match),
		"",
		GetCompilerFilename(meta.Tag, platform, compiler.Method),
		cacheDir,
	)
	if err != nil {
		return
	}

	method := download.ExtractFuncFromName(compiler.Method)
	if method == nil {
		err = errors.Errorf("invalid extract type: %s", compiler.Method)
		return
	}

	_, err = method(path, dir, compiler.Paths)
	if err != nil {
		err = errors.Wrapf(err, "failed to unzip package %s", path)
		return
	}

	return compiler, nil
}

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(
	ctx context.Context,
	gh *github.Client,
	config build.Config,
	dir string,
	platform string,
	cacheDir string,
) (compiler download.Compiler, err error) {
	resolved := config.Compiler.ResolveCompilerConfig()
	meta := versioning.DependencyMeta{
		Site: resolved.Site,
		User: resolved.User,
		Repo: resolved.Repo,
		Tag:  resolved.Version,
	}

	if meta.Tag == "" {
		meta.Tag = "v3.10.10"
	} else if meta.Tag[0] != 'v' {
		meta.Tag = "v" + meta.Tag
	}

	if meta.Site == "" {
		meta.Site = "github.com"
	}

	if meta.User == "" {
		meta.User = "pawn-lang"
	}

	if meta.Repo == "" {
		meta.Repo = "compiler"
	}

	compiler, hit, err := FromCache(meta, dir, platform, cacheDir)
	if err != nil {
		err = errors.Wrapf(err, "failed to get package %s from cache", resolved.Version)
		return
	}
	if hit {
		return
	}

	compiler, err = FromNet(ctx, gh, meta, dir, platform, cacheDir)
	if err != nil {
		err = errors.Wrapf(err, "failed to get package %s from net", resolved.Version)
		return
	}

	return compiler, nil
}

// GetCompilerPackageInfo returns the URL for a specific compiler version
func GetCompilerPackageInfo(cacheDir, platform string) (compiler download.Compiler, err error) {
	compilers, err := download.GetCompilerList(cacheDir)
	if err != nil {
		return
	}

	compiler, ok := compilers[platform]
	if !ok {
		err = errors.Errorf("no compiler for platform '%s'", platform)
	}
	return
}

// GetCompilerFilename returns the path to a compiler given its platform and
// version number.
func GetCompilerFilename(version, platform, method string) string {
	return fmt.Sprintf("pawn-%s-%s.%s", version, platform, method)
}
