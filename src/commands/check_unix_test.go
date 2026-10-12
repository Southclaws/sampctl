//go:build linux || darwin

package commands

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestCheckCompilerRejectsNonregularMetadata(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	resolved := build.Default().Compiler.ResolveCompilerConfig()
	dir := filepath.Join(cacheDir, "pawn", resolved.User, resolved.Repo, resolved.Version)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, syscall.Mkfifo(filepath.Join(cacheDir, "compilers.json"), 0o600))
	results, err := checkCompilers(context.Background(), pawnpackage.Package{}, checkOptions{CacheDir: cacheDir, Platform: "linux"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkSkipped, results[0].Status)
}

func TestCheckProjectRejectsNonregularConfiguration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pawn.json"), 0o600))
	_, result := checkProject(dir)
	assert.Equal(t, checkFailed, result.Status)
}
