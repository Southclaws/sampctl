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
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// FromCache attempts to get a compiler package from the cache, `hit` represents success
func FromCache(
	meta versioning.DependencyMeta,
	dir string,
	platform string,
	cacheDir string,
) (download.Compiler, bool, error) {
	fetcher, err := newCompilerPackageFetcher(meta, platform, cacheDir)
	if err != nil {
		return download.Compiler{}, false, err
	}
	return fetcher.fromCache(dir)
}

// FromNet downloads a compiler package to the cache
func FromNet(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	dir string,
	platform string,
	cacheDir string,
) (download.Compiler, error) {
	fetcher, err := newCompilerPackageFetcher(meta, platform, cacheDir)
	if err != nil {
		return download.Compiler{}, err
	}
	return fetcher.fromNetwork(ctx, gh, dir)
}

// GetCompilerPackage downloads and installs a Pawn compiler to a user directory
func GetCompilerPackage(
	ctx context.Context,
	gh *github.Client,
	resolved build.CompilerConfig,
	dir string,
	platform string,
	cacheDir string,
) (download.Compiler, error) {
	meta := versioning.DependencyMeta{
		Site: resolved.Site,
		User: resolved.User,
		Repo: resolved.Repo,
		Tag:  resolved.Version,
	}

	compiler, hit, err := FromCache(meta, dir, platform, cacheDir)
	if err != nil {
		return download.Compiler{}, errors.Wrapf(err, "failed to get package %s from cache", resolved.Version)
	}
	if hit {
		return compiler, nil
	}

	compiler, err = FromNet(ctx, gh, meta, dir, platform, cacheDir)
	if err != nil {
		return download.Compiler{}, errors.Wrapf(err, "failed to get package %s from net", resolved.Version)
	}

	return compiler, nil
}

type compilerPackageFetcher struct {
	meta     versioning.DependencyMeta
	platform string
	cacheDir string
	compiler download.Compiler
	extract  download.ExtractFunc
}

func newCompilerPackageFetcher(meta versioning.DependencyMeta, platform, cacheDir string) (*compilerPackageFetcher, error) {
	compiler, err := GetCompilerPackageInfo(cacheDir, platform)
	if err != nil {
		return nil, err
	}
	extract := download.ExtractFuncFromName(compiler.Method)
	if extract == nil {
		return nil, errors.Errorf("invalid extract type: %s", compiler.Method)
	}
	return &compilerPackageFetcher{
		meta:     meta,
		platform: platform,
		cacheDir: cacheDir,
		compiler: compiler,
		extract:  extract,
	}, nil
}

func (f *compilerPackageFetcher) fromCache(dir string) (download.Compiler, bool, error) {
	if f.meta.Tag == "" {
		return download.Compiler{}, false, nil
	}
	if !fs.Exists(dir) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return download.Compiler{}, false, errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	matcher := regexp.MustCompile(f.compiler.Match)
	res := infraresource.NewGitHubReleaseResource(f.meta, matcher, infraresource.ResourceTypeCompiler, nil)
	res.SetCacheDir(f.cacheDir)
	res.SetCacheTTL(0)

	print.Verb("Checking for cached compiler package", f.meta.Tag, "in", f.cacheDir)
	hit, assetPath := res.Cached(f.meta.Tag)
	if !hit {
		return download.Compiler{}, false, nil
	}

	files, err := f.extract(assetPath, dir, f.compiler.Paths)
	if err != nil {
		return download.Compiler{}, false, errors.Wrapf(err, "failed to extract package %s", assetPath)
	}

	if fs.IsPosixPlatform(f.platform) {
		print.Verb("setting permissions for binaries")
	}
	if err := fs.ChmodAllIfPosix(f.platform, files, fs.PermFileExec); err != nil {
		return download.Compiler{}, false, err
	}

	print.Verb("Using cached compiler package", f.meta.Tag)
	return f.compiler, true, nil
}

func (f *compilerPackageFetcher) fromNetwork(
	ctx context.Context,
	gh *github.Client,
	dir string,
) (download.Compiler, error) {
	print.Info("Downloading compiler package", f.meta.Tag)
	if !fs.Exists(dir) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return download.Compiler{}, errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	matcher := regexp.MustCompile(f.compiler.Match)
	res := infraresource.NewGitHubReleaseResource(f.meta, matcher, infraresource.ResourceTypeCompiler, gh)
	res.SetCacheDir(f.cacheDir)
	res.SetCacheTTL(0)

	requestedVersion := f.meta.Tag
	if requestedVersion == "" {
		requestedVersion = "latest"
	}

	if err := res.Ensure(ctx, requestedVersion, ""); err != nil {
		return download.Compiler{}, err
	}

	actualVersion := requestedVersion
	if actualVersion == "latest" {
		actualVersion = res.Version()
	}

	_, assetPath := res.Cached(actualVersion)
	if assetPath == "" {
		return download.Compiler{}, errors.New("failed to locate downloaded compiler asset")
	}

	files, err := f.extract(assetPath, dir, f.compiler.Paths)
	if err != nil {
		return download.Compiler{}, errors.Wrapf(err, "failed to extract package %s", assetPath)
	}

	if fs.IsPosixPlatform(f.platform) {
		print.Verb("setting permissions for binaries")
	}
	if err := fs.ChmodAllIfPosix(f.platform, files, fs.PermFileExec); err != nil {
		return download.Compiler{}, err
	}

	return f.compiler, nil
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
