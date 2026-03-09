package compiler

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

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
			err := os.MkdirAll(tt.args.cacheDir, 0o700)
			assert.NoError(t, err)

			_, err = FromNet(context.Background(), gh, tt.args.meta, dir, tt.args.platform, tt.args.cacheDir)
			assert.NoError(t, err)

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
			err := os.MkdirAll(tt.args.cacheDir, 0o700)
			assert.NoError(t, err)

			_, gotHit, err := FromCache(tt.args.meta, dir, tt.args.platform, tt.args.cacheDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, gotHit, tt.wantHit)
			assertCompilerLayout(t, dir, tt.args.platform)
		})
	}
}

func assertCompilerLayout(t *testing.T, dir, platform string) {
	t.Helper()

	switch platform {
	case "linux":
		assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc")))
		assert.True(t, fs.Exists(filepath.Join(dir, "lib", "libpawnc.so")))
	case "darwin":
		assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc")))
		assert.True(t, fs.Exists(filepath.Join(dir, "lib", "libpawnc.dylib")))
	case "windows":
		assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawncc.exe")))
		assert.True(t, fs.Exists(filepath.Join(dir, "bin", "pawnc.dll")))
	default:
		t.Fatalf("unsupported platform %s", platform)
	}
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
