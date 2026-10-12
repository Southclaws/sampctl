package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckManagedCacheMetadata(t *testing.T) {
	tests := []struct {
		name, file, contents string
		status               checkStatus
	}{
		{"valid compilers", "compilers.json", `{"linux":{"match":"pawnc.*","method":"tgz","binary":"pawncc","paths":{"bin/pawncc":"pawncc"}}}`, checkPassed},
		{"malformed compilers", "compilers.json", `{"linux":`, checkFailed},
		{"invalid compilers", "compilers.json", `{"linux":{"match":"[","method":"tgz","binary":"pawncc","paths":{"bin/pawncc":"pawncc"}}}`, checkFailed},
		{"escaping compiler target", "compilers.json", `{"linux":{"match":".*","method":"zip","binary":"pawncc","paths":{"bin/pawncc":"../user-file"}}}`, checkFailed},
		{"valid packages", "packages.json", `[]`, checkPassed},
		{"malformed packages", "packages.json", `[`, checkFailed},
		{"valid runtimes", "runtimes.json", `{"aliases":{},"packages":[]}`, checkPassed},
		{"malformed runtimes", "runtimes.json", `{"packages":`, checkFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := checkCacheTestDir(t)
			path := filepath.Join(dir, tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o600))
			results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: dir})
			require.NoError(t, err)
			require.Len(t, results, 2)
			assert.Equal(t, tt.status, results[1].Status)
			contents, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.contents, string(contents), "default diagnostics must preserve cache content")
		})
	}
}

func TestCheckManagedCacheInvalidationIsVerifiedAndIdempotent(t *testing.T) {
	dir := checkCacheTestDir(t)
	path := filepath.Join(dir, "compilers.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
	userPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(userPath, []byte("{"), 0o600))
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: dir, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, checkRepaired, results[1].Status)
	assert.True(t, results[1].RepairAttempted)
	assert.True(t, results[1].RepairVerified)
	assert.NoFileExists(t, path)
	assert.FileExists(t, userPath)
	results, err = checkManagedCache(context.Background(), checkOptions{CacheDir: dir, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status)
}

func TestCheckManagedCacheSymlinkNeverInvalidated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require additional privileges on Windows")
	}
	dir := checkCacheTestDir(t)
	external := filepath.Join(t.TempDir(), "user.json")
	require.NoError(t, os.WriteFile(external, []byte("{"), 0o600))
	link := filepath.Join(dir, "compilers.json")
	require.NoError(t, os.Symlink(external, link))
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: dir, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, checkFailed, results[1].Status)
	assert.False(t, results[1].CanFix)
	assert.FileExists(t, external)
	_, err = os.Lstat(link)
	assert.NoError(t, err)
}

func TestCheckManagedCacheSymlinkRootNeverInvalidated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require additional privileges on Windows")
	}
	dir := checkCacheTestDir(t)
	path := filepath.Join(dir, "compilers.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
	link := filepath.Join(t.TempDir(), "cache")
	require.NoError(t, os.Symlink(dir, link))
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: link, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.False(t, results[1].CanFix)
	assert.FileExists(t, path)
}

func TestCheckManagedCacheSymlinkAncestorNeverInvalidated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require additional privileges on Windows")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "cache")
	require.NoError(t, os.Mkdir(dir, 0o700))
	path := filepath.Join(dir, "compilers.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
	link := filepath.Join(t.TempDir(), "parent")
	require.NoError(t, os.Symlink(parent, link))
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: filepath.Join(link, "cache"), Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.False(t, results[1].CanFix)
	assert.FileExists(t, path)
}

func TestCheckManagedCacheWritePermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use POSIX permission bits")
	}
	dir := checkCacheTestDir(t)
	path := filepath.Join(dir, "packages.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0o700)) })
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: dir})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, checkWarning, results[0].Status)
	assert.Equal(t, checkPassed, results[1].Status, "readable metadata should still be checked")
}

func TestInvalidateCheckCacheMetadataRefusesChangedOrExternalFiles(t *testing.T) {
	dir := checkCacheTestDir(t)
	valid := filepath.Join(dir, "packages.json")
	require.NoError(t, os.WriteFile(valid, []byte("[]"), 0o600))
	err := invalidateCheckCacheMetadata(context.Background(), dir, valid)
	assert.Error(t, err)
	assert.FileExists(t, valid)
	external := filepath.Join(t.TempDir(), "packages.json")
	require.NoError(t, os.WriteFile(external, []byte("["), 0o600))
	err = invalidateCheckCacheMetadata(context.Background(), dir, external)
	assert.Error(t, err)
	assert.FileExists(t, external)
}

func TestCheckManagedCacheMissingAndInaccessible(t *testing.T) {
	dir := checkCacheTestDir(t)
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: filepath.Join(dir, "missing"), Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkSkipped, results[0].Status)
	assert.NoDirExists(t, filepath.Join(dir, "missing"))
	path := filepath.Join(dir, "not-directory")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	results, err = checkManagedCache(context.Background(), checkOptions{CacheDir: path})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
}

func TestCheckManagedCacheDanglingRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require additional privileges on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cache")
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing"), path))
	results, err := checkManagedCache(context.Background(), checkOptions{CacheDir: path, Fix: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Error(t, results[0].Err)
	_, err = os.Lstat(path)
	assert.NoError(t, err)
}

func TestCheckManagedCacheCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := checkManagedCache(ctx, checkOptions{CacheDir: t.TempDir(), Fix: true})
	assert.ErrorIs(t, err, context.Canceled)
}

func checkCacheTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}
