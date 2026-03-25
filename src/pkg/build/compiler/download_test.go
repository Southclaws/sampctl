package compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestGetCompilerPackage(t *testing.T) {
	t.Run("uses cached compiler package", func(t *testing.T) {
		meta := versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}
		rootDir := t.TempDir()
		cacheDir := filepath.Join(rootDir, "cache")
		dir := filepath.Join(rootDir, "compiler")
		seedCompilerCacheFixture(t, cacheDir, meta, "linux")

		compiler, err := GetCompilerPackage(context.Background(), CompilerFetchRequest{
			Meta: versioning.DependencyMeta{
				Site: "github.com",
				User: meta.User,
				Repo: meta.Repo,
				Tag:  meta.Tag,
			},
			Dir:      dir,
			Platform: "linux",
			CacheDir: cacheDir,
		})
		require.NoError(t, err)
		assert.Equal(t, "pawncc", compiler.Binary)
		assert.FileExists(t, filepath.Join(dir, "pawncc"))
	})

	t.Run("downloads compiler package when cache misses", func(t *testing.T) {
		meta := versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}
		rootDir := t.TempDir()
		cacheDir := filepath.Join(rootDir, "cache")
		dir := filepath.Join(rootDir, "compiler")
		pkg := seedCompilerCacheFixture(t, cacheDir, meta, "windows")
		assetName := offlineCompilerArchiveName("windows", meta.Tag)
		assetBody, err := os.ReadFile(cachedCompilerAssetPath(t, cacheDir, meta, pkg))
		require.NoError(t, err)
		client := newCompilerReleaseClient(t, meta, assetName, assetBody)

		// remove cached asset and manifest rewrite so network path is required
		require.NoError(t, os.Remove(cachedCompilerAssetPath(t, cacheDir, meta, pkg)))

		compiler, err := GetCompilerPackage(context.Background(), CompilerFetchRequest{
			GitHub: client,
			Meta: versioning.DependencyMeta{
				Site: "github.com",
				User: meta.User,
				Repo: meta.Repo,
				Tag:  meta.Tag,
			},
			Dir:      dir,
			Platform: "windows",
			CacheDir: cacheDir,
		})
		require.NoError(t, err)
		assert.Equal(t, "pawncc.exe", compiler.Binary)
		assert.FileExists(t, filepath.Join(dir, "pawncc.exe"))
	})
}

func Test_CompilerFromNet(t *testing.T) {
	tests := []struct {
		name          string
		meta          versioning.DependencyMeta
		platform      string
		expectedFiles []string
	}{
		{"linux-v3.10.8", versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "linux", []string{"pawncc", "libpawnc.so"}},
		{"darwin-v3.10.8", versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "darwin", []string{"pawncc", "libpawnc.dylib"}},
		{"windows-v3.10.8", versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "windows", []string{"pawncc.exe", "pawnc.dll"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			cacheDir := filepath.Join(rootDir, "cache")
			dir := filepath.Join(rootDir, "compiler")
			pkg := seedCompilerCacheFixture(t, cacheDir, tt.meta, tt.platform)
			assetName := offlineCompilerArchiveName(tt.platform, tt.meta.Tag)
			assetBody, err := os.ReadFile(cachedCompilerAssetPath(t, cacheDir, tt.meta, pkg))
			assert.NoError(t, err)

			client := newCompilerReleaseClient(t, tt.meta, assetName, assetBody)
			_, err = FromNet(context.Background(), CompilerFetchRequest{
				GitHub:   client,
				Meta:     tt.meta,
				Dir:      dir,
				Platform: tt.platform,
				CacheDir: cacheDir,
			})
			assert.NoError(t, err)

			for _, name := range tt.expectedFiles {
				assert.True(t, fs.Exists(filepath.Join(dir, name)))
			}
		})
	}
}

func Test_CompilerFromCache(t *testing.T) {
	tests := []struct {
		name          string
		meta          versioning.DependencyMeta
		platform      string
		expectedFiles []string
	}{
		{"linux-v3.10.8", versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "linux", []string{"pawncc", "libpawnc.so"}},
		{"darwin-v3.10.8", versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "darwin", []string{"pawncc", "libpawnc.dylib"}},
		{"windows-v3.10.8", versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"}, "windows", []string{"pawncc.exe", "pawnc.dll"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			cacheDir := filepath.Join(rootDir, "cache")
			dir := filepath.Join(rootDir, "compiler")

			seedCompilerCacheFixture(t, cacheDir, tt.meta, tt.platform)

			_, gotHit, err := FromCache(CompilerFetchRequest{
				Meta:     tt.meta,
				Dir:      dir,
				Platform: tt.platform,
				CacheDir: cacheDir,
			})
			assert.NoError(t, err)
			assert.True(t, gotHit)

			for _, name := range tt.expectedFiles {
				assert.True(t, fs.Exists(filepath.Join(dir, name)))
			}
		})
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

func TestCompilerPackageHelpers(t *testing.T) {
	t.Run("newCompilerPackageFetcher rejects invalid extract method", func(t *testing.T) {
		cacheDir := t.TempDir()
		manifest := download.Compilers{
			"linux": {Match: `compiler\.bin`, Method: "bogus", Binary: "pawncc", Paths: map[string]string{"pawncc": "pawncc"}},
		}
		require.NoError(t, download.WriteCompilerCacheFile(cacheDir, mustJSON(t, manifest)))

		_, err := newCompilerPackageFetcher(versioning.DependencyMeta{User: "u", Repo: "r", Tag: "v1"}, "linux", cacheDir)
		require.ErrorContains(t, err, "invalid extract type")
	})

	t.Run("GetCompilerPackageInfo returns platform error", func(t *testing.T) {
		cacheDir := t.TempDir()
		require.NoError(t, download.WriteCompilerCacheFile(cacheDir, mustJSON(t, download.Compilers{"linux": {Binary: "pawncc"}})))
		_, err := GetCompilerPackageInfo(cacheDir, "plan9")
		require.ErrorContains(t, err, "no compiler for platform")
	})

	t.Run("GetCompilerFilename formats filename", func(t *testing.T) {
		assert.Equal(t, "pawn-v3.10.11-linux.tgz", GetCompilerFilename("v3.10.11", "linux", "tgz"))
	})
}
