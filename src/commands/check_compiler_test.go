package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestCheckCustomCompiler(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, string)
		status checkStatus
	}{
		{"available", func(t *testing.T, dir string) {
			writeCheckCompilerFile(t, filepath.Join(dir, checkHostCompilerName()), 0o700)
		}, checkPassed},
		{"missing binary", func(*testing.T, string) {}, checkFailed},
		{"directory in place of binary", func(t *testing.T, dir string) {
			require.NoError(t, os.Mkdir(filepath.Join(dir, checkHostCompilerName()), 0o700))
		}, checkFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := checkCacheTestDir(t)
			tt.setup(t, dir)
			pkg := pawnpackage.Package{LocalPath: dir, Build: &build.Config{Compiler: build.CompilerConfig{Path: "."}}}

			results, err := checkCompilers(context.Background(), pkg, checkOptions{Dir: dir, CacheDir: filepath.Join(dir, "unused"), Platform: runtime.GOOS, Fix: true})
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, tt.status, results[0].Status)
			assert.False(t, results[0].CanFix)
			assert.NoDirExists(t, filepath.Join(dir, "unused"))
		})
	}
}

func TestCheckCustomCompilerNeverRepairsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use executable permission bits")
	}

	dir := checkCacheTestDir(t)
	binary := filepath.Join(dir, checkHostCompilerName())
	writeCheckCompilerFile(t, binary, 0o600)
	pkg := pawnpackage.Package{LocalPath: dir, Build: &build.Config{Compiler: build.CompilerConfig{Path: dir}}}

	results, err := checkCompilers(context.Background(), pkg, checkOptions{Dir: dir, Platform: runtime.GOOS, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.False(t, results[0].CanFix)

	info, err := os.Stat(binary)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCheckCustomCompilerBrokenDirectory(t *testing.T) {
	dir := checkCacheTestDir(t)
	path := filepath.Join(dir, "missing-compiler")
	pkg := pawnpackage.Package{LocalPath: dir, Build: &build.Config{Compiler: build.CompilerConfig{Path: path}}}

	results, err := checkCompilers(context.Background(), pkg, checkOptions{Dir: dir, Platform: runtime.GOOS, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.ErrorIs(t, results[0].Err, os.ErrNotExist)
	assert.False(t, results[0].CanFix)
	assert.NoDirExists(t, path)
}

func TestCheckManagedCompilerWithoutMetadataIsSkipped(t *testing.T) {
	pkg, opts, _, _ := checkCompilerFixture(t, "linux")
	metadata := filepath.Join(opts.CacheDir, "compilers.json")
	require.NoError(t, os.Remove(metadata))

	results, err := checkCompilers(context.Background(), pkg, opts)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkSkipped, results[0].Status)
	assert.False(t, results[0].CanFix)
	assert.NoFileExists(t, metadata)
}

func TestCheckManagedCompiler(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			pkg, opts, binary, library := checkCompilerFixture(t, platform)

			results, err := checkCompilers(context.Background(), pkg, opts)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, checkPassed, results[0].Status)

			require.NoError(t, os.Remove(library))

			results, err = checkCompilers(context.Background(), pkg, opts)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, checkFailed, results[0].Status)
			assert.False(t, results[0].CanFix)
			assert.FileExists(t, binary)
		})
	}
}

func TestCheckMissingManagedCompilerDoesNotCreateCache(t *testing.T) {
	dir := checkCacheTestDir(t)
	cacheDir := filepath.Join(dir, "cache")

	results, err := checkCompilers(context.Background(), pawnpackage.Package{LocalPath: dir}, checkOptions{Dir: dir, CacheDir: cacheDir, Platform: "linux"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.NoDirExists(t, cacheDir)
}

func TestCheckManagedCompilerRepairsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use executable permission bits")
	}

	pkg, opts, binary, _ := checkCompilerFixture(t, "linux")
	require.NoError(t, os.Chmod(binary, 0o640))

	results, err := checkCompilers(context.Background(), pkg, opts)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.True(t, results[0].CanFix)

	info, err := os.Stat(binary)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())

	opts.Fix = true
	results, err = checkCompilers(context.Background(), pkg, opts)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkRepaired, results[0].Status)
	assert.True(t, results[0].RepairAttempted)
	assert.True(t, results[0].RepairVerified)

	info, err = os.Stat(binary)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o751), info.Mode().Perm())

	results, err = checkCompilers(context.Background(), pkg, opts)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status)
	assert.False(t, results[0].RepairAttempted)
}

func TestCheckManagedCompilerDoesNotRepairSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require additional privileges on Windows")
	}

	pkg, opts, binary, _ := checkCompilerFixture(t, "linux")
	external := filepath.Join(t.TempDir(), "user-compiler")
	writeCheckCompilerFile(t, external, 0o600)
	require.NoError(t, os.Remove(binary))
	require.NoError(t, os.Symlink(external, binary))
	opts.Fix = true

	results, err := checkCompilers(context.Background(), pkg, opts)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.False(t, results[0].CanFix)

	info, err := os.Stat(external)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCheckManagedCompilerDoesNotRepairHardlink(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("hardlink permission checks require Linux or macOS")
	}

	pkg, opts, binary, _ := checkCompilerFixture(t, "linux")
	external := filepath.Join(t.TempDir(), "user-compiler")
	writeCheckCompilerFile(t, external, 0o600)
	require.NoError(t, os.Remove(binary))
	require.NoError(t, os.Link(external, binary))
	opts.Fix = true

	results, err := checkCompilers(context.Background(), pkg, opts)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.False(t, results[0].CanFix)
	assert.False(t, results[0].RepairAttempted)
	assert.Contains(t, results[0].Action, "manually")

	err = repairCheckCompilerPermissions(context.Background(), opts.CacheDir, binary)
	require.Error(t, err)

	info, err := os.Stat(external)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCheckCompilerBuildDefaults(t *testing.T) {
	dir := checkCacheTestDir(t)
	writeCheckCompilerFile(t, filepath.Join(dir, checkHostCompilerName()), 0o700)
	pkg := pawnpackage.Package{
		LocalPath: dir,
		Build:     &build.Config{Compiler: build.CompilerConfig{Path: dir}},
		Builds:    []*build.Config{{Name: "debug"}, {Name: "release"}},
	}

	results, err := checkCompilers(context.Background(), pkg, checkOptions{Dir: dir, Platform: runtime.GOOS})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	for _, result := range results {
		assert.Equal(t, checkPassed, result.Status)
	}
	assert.Empty(t, pkg.Builds[0].Compiler.Path, "checking must not mutate build definitions")
}

func TestCheckCustomCompilerUsesDiscoveredProjectRoot(t *testing.T) {
	dir := checkCacheTestDir(t)
	compilerDir := filepath.Join(dir, "compiler")
	writeCheckCompilerFile(t, filepath.Join(compilerDir, checkHostCompilerName()), 0o700)
	pkg := pawnpackage.Package{LocalPath: dir, Build: &build.Config{Compiler: build.CompilerConfig{Path: "compiler"}}}

	results, err := checkCompilers(context.Background(), pkg, checkOptions{Dir: filepath.Join(dir, "nested"), Platform: runtime.GOOS})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status)
}

func TestCheckCompilerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := checkCompilers(ctx, pawnpackage.Package{}, checkOptions{})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCheckCompilerUnsafeManagedPath(t *testing.T) {
	dir := checkCacheTestDir(t)
	pkg := pawnpackage.Package{LocalPath: dir, Build: &build.Config{Compiler: build.CompilerConfig{User: "../../user", Repo: "compiler"}}}

	results, err := checkCompilers(context.Background(), pkg, checkOptions{Dir: dir, CacheDir: filepath.Join(dir, "cache"), Platform: "linux", Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.False(t, results[0].CanFix)
}

func checkCompilerFixture(t *testing.T, platform string) (pawnpackage.Package, checkOptions, string, string) {
	t.Helper()
	dir := checkCacheTestDir(t)
	cacheDir := filepath.Join(dir, "cache")
	binaryName, libraryName := "pawncc", "libpawnc.so"
	if platform == "darwin" {
		libraryName = "libpawnc.dylib"
	} else if platform == "windows" {
		binaryName, libraryName = "pawncc.exe", "pawnc.dll"
	}

	metadata := download.Compilers{platform: {
		Match: "pawnc.*", Method: "zip", Binary: binaryName,
		Paths: map[string]string{"bin/" + binaryName: binaryName, "lib/" + libraryName: libraryName},
	}}
	require.NoError(t, fs.WriteJSONAtomic(filepath.Join(cacheDir, "compilers.json"), metadata, 0o700, 0o600))

	compilerDir := filepath.Join(cacheDir, "pawn", "pawn-lang", "compiler", "v3.10.10")
	binary, library := filepath.Join(compilerDir, binaryName), filepath.Join(compilerDir, libraryName)
	writeCheckCompilerFile(t, binary, 0o700)
	writeCheckCompilerFile(t, library, 0o600)

	return pawnpackage.Package{LocalPath: dir}, checkOptions{Dir: dir, CacheDir: cacheDir, Platform: platform}, binary, library
}

func checkHostCompilerName() string {
	if runtime.GOOS == "windows" {
		return "pawncc.exe"
	}
	return "pawncc"
}

func writeCheckCompilerFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("compiler fixture"), mode))
}
