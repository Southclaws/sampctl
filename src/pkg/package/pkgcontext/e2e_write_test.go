package pkgcontext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestWriteDefinitionPreservesExplicitRuntimeSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "pawn.json")
	config := `{
		"entry": "test.pwn",
		"output": "test.amx",
		"runtime": {"version": "0.3.7"},
		"dependencies": []
	}`
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o644))

	pkg, err := pawnpackage.PackageFromDir(tmpDir)
	require.NoError(t, err)
	pkg.LocalPath = tmpDir

	pkg.Dependencies = append(pkg.Dependencies, "test-user/test-package:1.0.0")

	err = pkg.WriteDefinition()
	require.NoError(t, err)

	writtenBytes, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var writtenConfig map[string]interface{}
	err = json.Unmarshal(writtenBytes, &writtenConfig)
	require.NoError(t, err)

	runtime, ok := writtenConfig["runtime"].(map[string]interface{})
	require.True(t, ok, "runtime should be a map")

	assert.Equal(t, "0.3.7", runtime["version"])

	deps, ok := writtenConfig["dependencies"].([]interface{})
	require.True(t, ok, "dependencies should be an array")
	require.Len(t, deps, 1)
	assert.Equal(t, "test-user/test-package:1.0.0", deps[0])

	defaultFields := []string{"port", "hostname", "maxplayers", "language", "mode", "rootLink", "rcon_password"}
	for _, field := range defaultFields {
		assert.NotContains(t, runtime, field)
	}
}
