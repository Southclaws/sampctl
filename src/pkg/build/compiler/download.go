package compiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type CompilerFetchRequest struct {
	GitHub   *github.Client
	Meta     versioning.DependencyMeta
	Dir      string
	Platform string
	CacheDir string
}

// FromCache installs a compiler from an existing cached archive.
func FromCache(request CompilerFetchRequest) (download.Compiler, bool, error) {
	return FromCacheContext(context.Background(), request)
}

// FromCacheContext installs a compiler from an existing cached archive.
func FromCacheContext(ctx context.Context, request CompilerFetchRequest) (pkg download.Compiler, hit bool, err error) {
	if pkg, hit, err := installedCompilerFromCache(ctx, request); hit || err != nil {
		return pkg, hit, err
	}
	unlock, err := lockCompilerInstallation(ctx, request.Dir)
	if err != nil {
		return pkg, false, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	fetcher, err := newCompilerPackageFetcherContext(ctx, request.Meta, request.Platform, request.CacheDir)
	if err != nil {
		return pkg, false, err
	}
	return fetcher.fromCache(ctx, request.Dir)
}

// FromNet downloads and installs a compiler package.
func FromNet(ctx context.Context, request CompilerFetchRequest) (pkg download.Compiler, err error) {
	if pkg, hit, err := installedCompilerFromCache(ctx, request); hit || err != nil {
		return pkg, err
	}
	unlock, err := lockCompilerInstallation(ctx, request.Dir)
	if err != nil {
		return pkg, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	fetcher, err := newCompilerPackageFetcherContext(ctx, request.Meta, request.Platform, request.CacheDir)
	if err != nil {
		return pkg, err
	}
	return fetcher.fromNetwork(ctx, request.GitHub, request.Dir)
}

// GetCompilerPackage installs a complete compiler, reusing verified cached files.
func GetCompilerPackage(ctx context.Context, request CompilerFetchRequest) (pkg download.Compiler, err error) {
	if pkg, hit, err := installedCompilerFromCache(ctx, request); hit || err != nil {
		return pkg, err
	}
	unlock, err := lockCompilerInstallation(ctx, request.Dir)
	if err != nil {
		return pkg, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	fetcher, err := newCompilerPackageFetcherContext(ctx, request.Meta, request.Platform, request.CacheDir)
	if err != nil {
		return pkg, err
	}
	pkg, hit, err := fetcher.fromCache(ctx, request.Dir)
	if err != nil {
		var archiveErr *compilerArchiveError
		if errors.As(err, &archiveErr) && ctx.Err() == nil {
			if cached, path := fetcher.resource(nil).Cached(request.Meta.Tag); cached && path == archiveErr.Path {
				if pathErr := compilerSafePath(filepath.Dir(path), false); pathErr != nil {
					return pkg, errors.Join(err, pathErr)
				}
				info, statErr := os.Lstat(path)
				if statErr != nil || !info.Mode().IsRegular() {
					return pkg, errors.Join(err, statErr)
				}
				if removeErr := os.Remove(path); removeErr != nil {
					return pkg, errors.Join(err, removeErr)
				}
				if syncErr := fs.SyncDir(filepath.Dir(path)); syncErr != nil {
					return pkg, errors.Join(err, syncErr)
				}
				pkg, networkErr := fetcher.fromNetwork(ctx, request.GitHub, request.Dir)
				if networkErr != nil {
					return pkg, errors.Join(err, networkErr)
				}
				return pkg, nil
			}
		}
		return pkg, fmt.Errorf("install cached compiler %s: %w", request.Meta.Tag, err)
	}
	if hit {
		return pkg, nil
	}
	return fetcher.fromNetwork(ctx, request.GitHub, request.Dir)
}

type compilerPackageFetcher struct {
	meta     versioning.DependencyMeta
	platform string
	cacheDir string
	compiler download.Compiler
}

func newCompilerPackageFetcher(meta versioning.DependencyMeta, platform, cacheDir string) (*compilerPackageFetcher, error) {
	return newCompilerPackageFetcherContext(context.Background(), meta, platform, cacheDir)
}

func newCompilerPackageFetcherContext(ctx context.Context, meta versioning.DependencyMeta, platform, cacheDir string) (*compilerPackageFetcher, error) {
	pkg, err := GetCompilerPackageInfoContext(ctx, cacheDir, platform)
	if err != nil {
		return nil, err
	}
	if err := validateCompilerPackage(pkg); err != nil {
		return nil, err
	}
	return &compilerPackageFetcher{meta: meta, platform: platform, cacheDir: cacheDir, compiler: pkg}, nil
}

func (f *compilerPackageFetcher) resource(gh *github.Client) *infraresource.GitHubReleaseResource {
	matcher := regexp.MustCompile(f.compiler.Match)
	res := infraresource.NewGitHubReleaseResource(f.meta, matcher, infraresource.ResourceTypeCompiler, gh)
	res.SetCacheDir(f.cacheDir)
	res.SetCacheTTL(0)
	return res
}

func (f *compilerPackageFetcher) fromCache(ctx context.Context, dir string) (download.Compiler, bool, error) {
	if err := recoverCompilerInstallation(ctx, dir); err != nil {
		return download.Compiler{}, false, err
	}
	if f.meta.Tag == "" {
		return download.Compiler{}, false, nil
	}
	if err := verifyCompilerInstallation(ctx, dir, f.meta, f.platform, f.compiler); err == nil {
		print.Verb("Using installed compiler", f.meta.Tag)
		return f.compiler, true, nil
	}
	hit, assetPath := f.resource(nil).Cached(f.meta.Tag)
	if !hit {
		return download.Compiler{}, false, nil
	}
	if err := installCompilerArchive(ctx, assetPath, dir, f.meta, f.platform, f.compiler); err != nil {
		return download.Compiler{}, false, err
	}
	return f.compiler, true, nil
}

func (f *compilerPackageFetcher) fromNetwork(ctx context.Context, gh *github.Client, dir string) (download.Compiler, error) {
	if err := recoverCompilerInstallation(ctx, dir); err != nil {
		return download.Compiler{}, err
	}
	if err := verifyCompilerInstallation(ctx, dir, f.meta, f.platform, f.compiler); err == nil {
		return f.compiler, nil
	}
	print.Info("Downloading compiler package", f.meta.Tag)
	res := f.resource(gh)
	requested := f.meta.Tag
	if requested == "" {
		requested = "latest"
	}
	if err := res.Ensure(ctx, requested, ""); err != nil {
		return download.Compiler{}, err
	}
	actual := requested
	if actual == "latest" {
		actual = res.Version()
	}
	_, assetPath := res.Cached(actual)
	if assetPath == "" {
		return download.Compiler{}, errors.New("downloaded compiler archive is unavailable")
	}
	if err := installCompilerArchive(ctx, assetPath, dir, f.meta, f.platform, f.compiler); err != nil {
		return download.Compiler{}, err
	}
	return f.compiler, nil
}

func compilerPackageInstalled(dir string, pkg download.Compiler) bool {
	return validateCompilerFiles(dir, pkg) == nil
}

// GetCompilerPackageInfo returns compiler distribution metadata for a platform.
func GetCompilerPackageInfo(cacheDir, platform string) (download.Compiler, error) {
	return GetCompilerPackageInfoContext(context.Background(), cacheDir, platform)
}

// GetCompilerPackageInfoContext returns compiler distribution metadata for a platform.
func GetCompilerPackageInfoContext(ctx context.Context, cacheDir, platform string) (download.Compiler, error) {
	compilers, err := download.GetCompilerListContext(ctx, cacheDir)
	if err != nil {
		return download.Compiler{}, err
	}
	pkg, ok := compilers[platform]
	if !ok {
		return download.Compiler{}, fmt.Errorf("no compiler for platform %q", platform)
	}
	return pkg, nil
}

// GetCompilerFilename returns the archive filename for a compiler version.
func GetCompilerFilename(version, platform, method string) string {
	return fmt.Sprintf("pawn-%s-%s.%s", version, platform, method)
}
