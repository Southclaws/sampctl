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

func (f *compilerPackageFetcher) archiveName() string {
	return GetCompilerFilename(f.meta.Tag, f.platform, f.compiler.Method)
}

func (f *compilerPackageFetcher) fromCache(dir string) (download.Compiler, bool, error) {
	filename := f.archiveName()
	print.Verb("Checking for cached package", filename, "in", f.cacheDir)
	hit, err := download.FromCache(f.cacheDir, filename, dir, f.extract, f.compiler.Paths, f.platform)
	if !hit || err != nil {
		return download.Compiler{}, hit, err
	}
	print.Verb("Using cached package", filename)
	return f.compiler, true, nil
}

func (f *compilerPackageFetcher) fromNetwork(
	ctx context.Context,
	gh *github.Client,
	dir string,
) (download.Compiler, error) {
	print.Info("Downloading compiler package", f.meta.Tag)
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return download.Compiler{}, errors.Wrapf(err, "failed to create dir %s", dir)
		}
	}

	assetPath, _, err := download.ReleaseAssetByPattern(
		ctx,
		gh,
		f.meta,
		regexp.MustCompile(f.compiler.Match),
		"",
		f.archiveName(),
		f.cacheDir,
	)
	if err != nil {
		return download.Compiler{}, err
	}

	if _, err = f.extract(assetPath, dir, f.compiler.Paths); err != nil {
		return download.Compiler{}, errors.Wrapf(err, "failed to unzip package %s", assetPath)
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
