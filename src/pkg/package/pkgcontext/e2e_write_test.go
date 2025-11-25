package pkgcontext

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

// TestE2EConfigWrite simulates the full workflow of loading and writing a package
// to verify that default runtime fields are not polluting the config file
func TestE2EConfigWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "sampctl-e2e-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "pawn.json")
	minimalConfig := map[string]interface{}{
		"entry":  "test.pwn",
		"output": "test.amx",
		"runtime": map[string]interface{}{
			"version": "0.3.7",
		},
		"dependencies": []string{},
	}

	configBytes, err := json.MarshalIndent(minimalConfig, "", "\t")
	require.NoError(t, err)

	err = ioutil.WriteFile(configPath, configBytes, 0o644)
	require.NoError(t, err)

	t.Log("Initial config:")
	t.Log(string(configBytes))

	pkg, err := pawnpackage.PackageFromDir(tmpDir)
	require.NoError(t, err)
	pkg.LocalPath = tmpDir

	pkg.Dependencies = append(pkg.Dependencies, "test-user/test-package:1.0.0")

	err = pkg.WriteDefinition()
	require.NoError(t, err)

	writtenBytes, err := ioutil.ReadFile(configPath)
	require.NoError(t, err)

	var writtenConfig map[string]interface{}
	err = json.Unmarshal(writtenBytes, &writtenConfig)
	require.NoError(t, err)

	t.Log("Final config:")
	prettyBytes, _ := json.MarshalIndent(writtenConfig, "", "\t")
	t.Log(string(prettyBytes))

	runtime, ok := writtenConfig["runtime"].(map[string]interface{})
	require.True(t, ok, "runtime should be a map")

	assert.Equal(t, "0.3.7", runtime["version"])

	deps, ok := writtenConfig["dependencies"].([]interface{})
	require.True(t, ok, "dependencies should be an array")
	assert.Len(t, deps, 1)
	assert.Equal(t, "test-user/test-package:1.0.0", deps[0])

	defaultFields := []string{"port", "hostname", "maxplayers", "language", "mode", "rootLink", "rcon_password"}
	for _, field := range defaultFields {
		_, has := runtime[field]
		assert.False(t, has, "runtime should not have default field: %s", field)
	}
}
