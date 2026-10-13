package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDependencyStagingPreservesRecoveryFiles(t *testing.T) {
	t.Parallel()
	pkg := newCheckDependenciesPackage(t)
	require.NoError(t, os.WriteFile(filepath.Join(pkg.LocalPath, "pawn.json"), []byte("{}"), 0o600))
	stage := filepath.Join(pkg.Vendor, ".sampctl-stage-interrupted")
	backup := filepath.Join(stage, ".previous-fixture", "dependency")
	require.NoError(t, os.MkdirAll(backup, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(backup, "keep.inc"), []byte("previous installation"), 0o600))
	before := snapshotCheckDependency(t, stage)
	for _, fix := range []bool{false, true} {
		results, err := runChecks(context.Background(), checkOptions{Dir: pkg.LocalPath, CacheDir: t.TempDir(), Platform: "linux", Fix: fix})
		require.NoError(t, err)
		var found bool
		for _, result := range results {
			if result.Name != "dependency staging" {
				continue
			}
			found = true
			assert.Equal(t, checkWarning, result.Status)
			assert.Equal(t, stage, result.Path)
			assert.False(t, result.CanFix)
			assert.False(t, result.RepairAttempted)
			assert.Contains(t, result.Action, "backup")
		}
		assert.True(t, found)
		assert.Equal(t, before, snapshotCheckDependency(t, stage))
	}
}

func TestCheckDependencyStagingIgnoresExternalSymlinks(t *testing.T) {
	t.Parallel()
	pkg := newCheckDependenciesPackage(t)
	require.NoError(t, os.Mkdir(pkg.Vendor, 0o700))
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "keep.inc"), []byte("external"), 0o600))
	if err := os.Symlink(external, filepath.Join(pkg.Vendor, ".sampctl-stage-external")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	results, err := checkDependencies(context.Background(), pkg, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status)
}
