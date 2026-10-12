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

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestCheckDependenciesRejectsSpecialFiles(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
	path := filepath.Join(pkg.Vendor, "repo")
	newCheckDependencyRepo(t, path, "")
	source := filepath.Join(path, "library.inc")
	require.NoError(t, os.Remove(source))
	require.NoError(t, syscall.Mkfifo(source, 0o600))
	results, err := checkDependencies(context.Background(), pkg, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Error(t, results[0].Err)
	info, err := os.Stat(source)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeNamedPipe)
}

func TestCheckDependenciesRejectsSpecialMetadata(t *testing.T) {
	for _, name := range []string{"HEAD", "config"} {
		t.Run(name, func(t *testing.T) {
			pkg := newCheckDependenciesPackage(t)
			pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
			path := filepath.Join(pkg.Vendor, "repo")
			newCheckDependencyRepo(t, path, "")
			metadata := filepath.Join(path, ".git", name)
			require.NoError(t, os.Remove(metadata))
			require.NoError(t, syscall.Mkfifo(metadata, 0o600))
			results, err := checkDependencies(context.Background(), pkg, nil, nil)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, checkFailed, results[0].Status)
			assert.Error(t, results[0].Err)
		})
	}
}
