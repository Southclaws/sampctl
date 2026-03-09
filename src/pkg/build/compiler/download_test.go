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
			_, err = FromNet(context.Background(), client, tt.meta, dir, tt.platform, cacheDir)
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

			_, gotHit, err := FromCache(tt.meta, dir, tt.platform, cacheDir)
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
