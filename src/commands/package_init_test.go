package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInitDirectoryAllowsEmptyDirectory(t *testing.T) {
	t.Parallel()

	err := validateInitDirectory(t.TempDir())
	require.NoError(t, err)
}

func TestValidateInitDirectoryRejectsExistingPackage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(`{"runtime":{"version":"0.3.7"}}`), 0o644))

	err := validateInitDirectory(dir)
	require.Error(t, err)
	assert.EqualError(t, err, "Directory already appears to be a package")
}

func TestValidateInitDirectoryRejectsInvalidPackageDefinitionState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(`{"runtime":{"version":"0.3.7"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.yaml"), []byte("runtime:\n  version: 0.3.7\n"), 0o644))

	err := validateInitDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect package definition")
}
