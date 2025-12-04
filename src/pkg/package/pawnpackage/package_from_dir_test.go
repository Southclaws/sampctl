package pawnpackage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleJSON = `{
  "runtime": {
    "version": "json-version"
  }
}`

const sampleYAML = "runtime:\n  version: yaml-version\n"

func TestPackageFromDirErrorsWhenBothConfigsExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(sampleJSON), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.yaml"), []byte(sampleYAML), 0o644))

	_, err := PackageFromDir(dir)
	require.Error(t, err)
}

func TestPackageFromDirLoadsJson(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(sampleJSON), 0o644))

	pkg, err := PackageFromDir(dir)
	require.NoError(t, err)
	require.Equal(t, "json", pkg.Format)
	require.NotNil(t, pkg.Runtime)
	require.Equal(t, "json-version", pkg.Runtime.Version)
}

func TestPackageFromDirLoadsYaml(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.yaml"), []byte(sampleYAML), 0o644))

	pkg, err := PackageFromDir(dir)
	require.NoError(t, err)
	require.Equal(t, "yaml", pkg.Format)
	require.NotNil(t, pkg.Runtime)
	require.Equal(t, "yaml-version", pkg.Runtime.Version)
}
