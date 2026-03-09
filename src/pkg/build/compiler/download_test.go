package compiler

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/md5"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

const compilerTestCacheFixtureDir = "tests/cache"

func Test_CompilerFromNet(t *testing.T) {
	type args struct {
		meta     versioning.DependencyMeta
		platform string
		cacheDir string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"linux-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "linux", "tests/cache"}, false},
		{"darwin-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "darwin", "tests/cache"}, false},
		{"windows-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "windows", "tests/cache"}, false},
		{"linux-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "linux", "tests/cache"}, false},
		{"darwin-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "darwin", "tests/cache"}, false},
		{"windows-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "windows", "tests/cache"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cacheDir := prepareCompilerTestCache(t, false)

			_, err := FromNet(context.Background(), gh, tt.args.meta, dir, tt.args.platform, cacheDir)
			require.NoError(t, err)

			assertCompilerLayout(t, dir, tt.args.platform)
		})
	}
}

func Test_CompilerFromCache(t *testing.T) {
	type args struct {
		meta     versioning.DependencyMeta
		platform string
		cacheDir string
	}
	tests := []struct {
		name    string
		args    args
		wantHit bool
		wantErr bool
	}{
		{"linux-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "linux", "tests/cache"}, true, false},
		{"darwin-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "darwin", "tests/cache"}, true, false},
		{"windows-v3.10.4", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.4"}, "windows", "tests/cache"}, true, false},
		{"linux-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "linux", "tests/cache"}, true, false},
		{"darwin-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "darwin", "tests/cache"}, true, false},
		{"windows-v3.10.8", args{versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "windows", "tests/cache"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cacheDir := prepareCompilerTestCache(t, false)
			require.NoError(t, seedCompilerCacheAsset(t, cacheDir, tt.args.meta, tt.args.platform))

			_, gotHit, err := FromCache(tt.args.meta, dir, tt.args.platform, cacheDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantHit, gotHit)
			assertCompilerLayout(t, dir, tt.args.platform)
		})
	}
}

func assertCompilerLayout(t *testing.T, dir, platform string) {
	t.Helper()

	var pkg download.Compiler
	switch platform {
	case "linux":
		pkg = download.Compiler{
			Binary: "bin/pawncc",
			Paths: map[string]string{
				"bin/pawncc":      "bin/pawncc",
				"lib/libpawnc.so": "lib/libpawnc.so",
			},
			PreserveLayout: true,
		}
		assert.True(t, compilerPathExists(dir, "bin/pawncc"))
		assert.True(t, compilerPathExists(dir, "lib/libpawnc.so"))
	case "darwin":
		pkg = download.Compiler{
			Binary: "bin/pawncc",
			Paths: map[string]string{
				"bin/pawncc":         "bin/pawncc",
				"lib/libpawnc.dylib": "lib/libpawnc.dylib",
			},
			PreserveLayout: true,
		}
		assert.True(t, compilerPathExists(dir, "bin/pawncc"))
		assert.True(t, compilerPathExists(dir, "lib/libpawnc.dylib"))
	case "windows":
		pkg = download.Compiler{
			Binary: "bin/pawncc.exe",
			Paths: map[string]string{
				"bin/pawncc.exe": "bin/pawncc.exe",
				"bin/pawnc.dll":  "bin/pawnc.dll",
			},
			PreserveLayout: true,
		}
		assert.True(t, compilerPathExists(dir, "bin/pawncc.exe"))
		assert.True(t, compilerPathExists(dir, "bin/pawnc.dll"))
	default:
		t.Fatalf("unsupported platform %s", platform)
	}

	assert.True(t, compilerPackageInstalled(dir, pkg))
}

func prepareCompilerTestCache(t *testing.T, withAssets bool) string {
	t.Helper()

	cacheDir := t.TempDir()
	compilerList, err := os.ReadFile(filepath.Join(compilerTestCacheFixtureDir, "compilers.json"))
	require.NoError(t, err)
	require.NoError(t, download.WriteCompilerCacheFile(cacheDir, compilerList))

	if withAssets {
		require.NoError(t, copyDir(filepath.Join(compilerTestCacheFixtureDir, "compiler"), filepath.Join(cacheDir, "compiler")))
	}

	return cacheDir
}

func seedCompilerCacheAsset(t *testing.T, cacheDir string, meta versioning.DependencyMeta, platform string) error {
	t.Helper()

	compiler, err := GetCompilerPackageInfo(cacheDir, platform)
	if err != nil {
		return err
	}

	matcher := regexp.MustCompile(compiler.Match)
	cachePath := compilerAssetCachePath(cacheDir, meta, matcher)
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return err
	}

	archivePath := filepath.Join(cachePath, fmt.Sprintf("fixture.%s", compiler.Method))
	switch platform {
	case "linux":
		return writeTarGzArchive(archivePath, map[string]string{
			"pawnc-v1-linux/bin/pawncc":      "exe",
			"pawnc-v1-linux/lib/libpawnc.so": "lib",
		})
	case "darwin":
		return writeZipArchive(archivePath, map[string]string{
			"pawnc-v1-darwin/bin/pawncc":         "exe",
			"pawnc-v1-darwin/lib/libpawnc.dylib": "lib",
		})
	case "windows":
		return writeZipArchive(archivePath, map[string]string{
			"pawnc-v1-windows/bin/pawncc.exe": "exe",
			"pawnc-v1-windows/bin/pawnc.dll":  "dll",
		})
	default:
		return fmt.Errorf("unsupported platform %s", platform)
	}
}

func compilerAssetCachePath(cacheDir string, meta versioning.DependencyMeta, matcher *regexp.Regexp) string {
	identifier := filepath.Join("github.com", meta.User, meta.Repo)
	if matcher != nil {
		sum := md5.Sum([]byte(matcher.String()))
		identifier = fmt.Sprintf("%s-%x", identifier, sum[:4])
	}
	cacheSum := md5.Sum([]byte(identifier + ":" + meta.Tag))
	return filepath.Join(cacheDir, string(infraresource.ResourceTypeCompiler), identifier, meta.Tag, fmt.Sprintf("%x", cacheSum[:8]))
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		return util.CopyFile(path, target)
	})
}

func TestCompilerPackageInstalled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkg := download.Compiler{
		Binary: "pawncc",
		Paths: map[string]string{
			"pawncc":    "pawncc",
			"pawnc.dll": "pawnc.dll",
		},
	}

	require.False(t, compilerPackageInstalled(dir, pkg))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawncc"), []byte("bin"), 0o755))
	require.False(t, compilerPackageInstalled(dir, pkg))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawnc.dll"), []byte("dll"), 0o644))
	require.True(t, compilerPackageInstalled(dir, pkg))
}

func TestCompilerPackageInstalledPreservedLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkg := download.Compiler{
		Binary: "bin/pawncc.exe",
		Paths: map[string]string{
			"bin/pawncc.exe": "bin/pawncc.exe",
			"bin/pawnc.dll":  "bin/pawnc.dll",
		},
		PreserveLayout: true,
	}

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "pawncc.exe"), []byte("bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "pawnc.dll"), []byte("dll"), 0o644))
	require.True(t, compilerPackageInstalled(dir, pkg))
}

func TestCompilerExecutablePathFallsBackToFlatLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawncc"), []byte("bin"), 0o755))

	assert.Equal(
		t,
		filepath.Join(dir, "pawncc"),
		compilerExecutablePath(dir, filepath.Join("bin", "pawncc")),
	)
}

func TestCompilerFromCustomPathPreservesRelativeBinaryPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binaryName := "pawncc"
	if runtime.GOOS == "windows" {
		binaryName = "pawncc.exe"
	}

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", binaryName), []byte("bin"), 0o755))

	pkg, err := compilerFromCustomPath(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("bin", binaryName), pkg.Binary)
	assert.Equal(t, filepath.Join(dir, "bin", binaryName), compilerExecutablePath(dir, pkg.Binary))
}

func TestExtractCompilerPackagePreservesArchiveLayout(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "compiler.zip")
	require.NoError(t, writeZipArchive(archive, map[string]string{
		"pawnc-v1-windows/bin/pawncc.exe": "exe",
		"pawnc-v1-windows/bin/pawnc.dll":  "dll",
		"pawnc-v1-windows/bin/mylib.dll":  "extra",
	}))

	fetcher := &compilerPackageFetcher{
		compiler: download.Compiler{
			Method:         download.ExtractZip,
			Binary:         "bin/pawncc.exe",
			PreserveLayout: true,
		},
	}

	dir := t.TempDir()
	files, err := fetcher.extractCompilerPackage(archive, dir)
	require.NoError(t, err)
	assert.Contains(t, files, "pawnc-v1-windows/bin/mylib.dll")
	assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc.exe")))
	assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawnc.dll")))
	assert.True(t, fs.Exists(filepath.Join(dir, "bin", "mylib.dll")))
	assert.False(t, fs.Exists(filepath.Join(dir, "pawnc-v1-windows")))
	assert.Equal(t, filepath.Join(dir, "bin", "mylib.dll"), files["pawnc-v1-windows/bin/mylib.dll"])
	assert.True(t, compilerPackageInstalled(dir, download.Compiler{
		Binary: "bin/pawncc.exe",
		Paths: map[string]string{
			"bin/pawncc.exe": "bin/pawncc.exe",
			"bin/pawnc.dll":  "bin/pawnc.dll",
		},
		PreserveLayout: true,
	}))
}

func TestExtractCompilerPackagePreservesTarLayout(t *testing.T) {
	t.Parallel()

	archive := filepath.Join(t.TempDir(), "compiler.tar.gz")
	require.NoError(t, writeTarGzArchive(archive, map[string]string{
		"pawnc-v1-linux/bin/pawncc":      "exe",
		"pawnc-v1-linux/lib/libpawnc.so": "lib",
		"pawnc-v1-linux/lib/libextra.so": "extra",
	}))

	fetcher := &compilerPackageFetcher{
		compiler: download.Compiler{
			Method:         download.ExtractTgz,
			Binary:         "bin/pawncc",
			PreserveLayout: true,
		},
	}

	dir := t.TempDir()
	files, err := fetcher.extractCompilerPackage(archive, dir)
	require.NoError(t, err)
	assert.Contains(t, files, "pawnc-v1-linux/lib/libextra.so")
	assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc")))
	assert.True(t, fs.Exists(filepath.Join(dir, "lib", "libpawnc.so")))
	assert.True(t, fs.Exists(filepath.Join(dir, "lib", "libextra.so")))
	assert.False(t, fs.Exists(filepath.Join(dir, "pawnc-v1-linux")))
}

func writeZipArchive(filename string, files map[string]string) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := zip.NewWriter(out)
	defer writer.Close()

	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}

func writeTarGzArchive(filename string, files map[string]string) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	gzw := gzip.NewWriter(out)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}
