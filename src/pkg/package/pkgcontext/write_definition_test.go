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

// TestInstallDoesNotWriteDefaultRuntimeFields verifies that installing a package
// does not add default runtime fields to the config file
func TestInstallDoesNotWriteDefaultRuntimeFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "sampctl-install-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

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

	configPath := filepath.Join(tmpDir, "pawn.json")
	err = ioutil.WriteFile(configPath, configBytes, 0o644)
	require.NoError(t, err)

	// Load the package
	pkg, err := pawnpackage.PackageFromDir(tmpDir)
	require.NoError(t, err)
	pkg.LocalPath = tmpDir

	assert.NotNil(t, pkg.Runtime)
	assert.Equal(t, "0.3.7", pkg.Runtime.Version)

	pkg.Dependencies = append(pkg.Dependencies, "test-user/test-package:1.0.0")

	err = pkg.WriteDefinition()
	require.NoError(t, err)

	writtenBytes, err := ioutil.ReadFile(configPath)
	require.NoError(t, err)

	var writtenConfig map[string]interface{}
	err = json.Unmarshal(writtenBytes, &writtenConfig)
	require.NoError(t, err)

	deps, ok := writtenConfig["dependencies"].([]interface{})
	require.True(t, ok, "dependencies should be an array")
	require.Len(t, deps, 1)
	assert.Equal(t, "test-user/test-package:1.0.0", deps[0].(string))

	runtime, ok := writtenConfig["runtime"].(map[string]interface{})
	require.True(t, ok, "runtime should be a map")
	assert.Equal(t, "0.3.7", runtime["version"].(string))

	t.Logf("Runtime fields in written config: %v", runtime)

	_, hasMode := runtime["mode"]
	_, hasPort := runtime["port"]
	_, hasRCON := runtime["rcon_password"]
	_, hasHostname := runtime["hostname"]
	_, hasMaxPlayers := runtime["maxplayers"]
	_, hasLanguage := runtime["language"]
	_, hasRootLink := runtime["rootLink"]

	assert.False(t, hasMode, "mode should not be written to config (it's a default value)")
	assert.False(t, hasPort, "port should not be written to config (it's a default value)")
	assert.False(t, hasRCON, "rcon_password should not be written to config (it's a default value)")
	assert.False(t, hasHostname, "hostname should not be written to config (it's a default value)")
	assert.False(t, hasMaxPlayers, "maxplayers should not be written to config (it's a default value)")
	assert.False(t, hasLanguage, "language should not be written to config (it's a default value)")
	assert.False(t, hasRootLink, "rootLink should not be written to config (it's a default value)")
}

// TestPackageContextDoesNotApplyDefaultsToPackageRuntime verifies that
// NewPackageContext does not populate default runtime values in pcx.Package.Runtime
func TestPackageContextDoesNotApplyDefaultsToPackageRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "sampctl-context-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	minimalConfig := map[string]interface{}{
		"entry":  "test.pwn",
		"output": "test.amx",
		"runtime": map[string]interface{}{
			"version": "0.3.7",
		},
	}

	configBytes, err := json.MarshalIndent(minimalConfig, "", "\t")
	require.NoError(t, err)

	configPath := filepath.Join(tmpDir, "pawn.json")
	err = ioutil.WriteFile(configPath, configBytes, 0o644)
	require.NoError(t, err)

	cacheDir, err := os.MkdirTemp("", "sampctl-cache-*")
	require.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	pcx, err := NewPackageContext(nil, nil, true, tmpDir, "linux", cacheDir, "", false)
	require.NoError(t, err)

	assert.NotNil(t, pcx.Package.Runtime, "Runtime should be initialized")
	assert.Equal(t, "0.3.7", pcx.Package.Runtime.Version, "Version should be preserved")

	assert.Nil(t, pcx.Package.Runtime.Port, "Port should remain nil (not defaulted)")
	assert.Nil(t, pcx.Package.Runtime.RCONPassword, "RCONPassword should remain nil (not defaulted)")
	assert.Equal(t, "", string(pcx.Package.Runtime.Mode), "Mode should remain empty (not defaulted)")
	assert.Nil(t, pcx.Package.Runtime.Hostname, "Hostname should remain nil (not defaulted)")
	assert.Nil(t, pcx.Package.Runtime.MaxPlayers, "MaxPlayers should remain nil (not defaulted)")
}

// TestActualRuntimeHasDefaults verifies that ActualRuntime (used for execution)
// does get defaults applied when needed
func TestActualRuntimeHasDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "sampctl-actual-runtime-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	minimalConfig := map[string]interface{}{
		"entry":  "test.pwn",
		"output": "test.amx",
		"local":  true,
		"runtime": map[string]interface{}{
			"version": "0.3.7",
		},
	}

	configBytes, err := json.MarshalIndent(minimalConfig, "", "\t")
	require.NoError(t, err)

	configPath := filepath.Join(tmpDir, "pawn.json")
	err = ioutil.WriteFile(configPath, configBytes, 0o644)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "test.pwn"), []byte("main() {}"), 0o644)
	require.NoError(t, err)

	cacheDir, err := os.MkdirTemp("", "sampctl-cache-*")
	require.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	pcx, err := NewPackageContext(nil, nil, true, tmpDir, "linux", cacheDir, "", false)
	require.NoError(t, err)

	pcx.ActualRuntime = *pcx.Package.Runtime

	assert.Nil(t, pcx.ActualRuntime.Port, "ActualRuntime.Port should start as nil")
}
