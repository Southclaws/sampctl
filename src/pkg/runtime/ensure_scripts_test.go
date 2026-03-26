package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsureScripts(t *testing.T) {
	t.Parallel()

	t.Run("succeeds when declared scripts exist", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "gamemodes"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "filterscripts"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(workingDir, "gamemodes", "main.amx"), []byte("amx"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(workingDir, "filterscripts", "test.amx"), []byte("amx"), 0o644))

		err := EnsureScripts(run.Runtime{
			WorkingDir:    workingDir,
			Gamemodes:     []string{"main"},
			Filterscripts: []string{"test"},
		})
		assert.NoError(t, err)
	})

	t.Run("returns combined missing script errors", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "gamemodes"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "filterscripts"), 0o755))

		err := EnsureScripts(run.Runtime{
			WorkingDir:    workingDir,
			Gamemodes:     []string{"missing-main"},
			Filterscripts: []string{"missing-filter"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gamemode 'missing-main' is missing")
		assert.Contains(t, err.Error(), "filterscript 'missing-filter' is missing")
	})
}
