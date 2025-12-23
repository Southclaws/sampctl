package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

func TestSAMPConfigGeneration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "samp_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create necessary directories for SA-MP
	err = os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0o755)
	require.NoError(t, err)

	// Create a SA-MP runtime config
	cfg := &run.Runtime{
		WorkingDir: tmpDir,
		Platform:   "linux",
		Version:    "0.3.7",
		Mode:       run.Server,
	}

	// Set some basic config values
	hostname := "My SA-MP Test Server"
	port := 7777
	maxPlayers := 100
	rconPassword := "testpass123"

	cfg.Hostname = &hostname
	cfg.Port = &port
	cfg.MaxPlayers = &maxPlayers
	cfg.RCONPassword = &rconPassword
	cfg.Gamemodes = []string{"test"}
	cfg.Filterscripts = []string{"admin"}
	cfg.Plugins = []run.Plugin{"mysql", "streamer"}

	// Test that it's detected as SA-MP
	assert.False(t, cfg.IsOpenMP())

	err = GenerateConfig(cfg)
	require.NoError(t, err)

	// Check that server.cfg was created
	serverCfgPath := filepath.Join(tmpDir, "server.cfg")
	assert.FileExists(t, serverCfgPath)

	// Read the file and check some basic content
	content, err := os.ReadFile(serverCfgPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "hostname My SA-MP Test Server")
	assert.Contains(t, contentStr, "port 7777")
	assert.Contains(t, contentStr, "maxplayers 100")
	assert.Contains(t, contentStr, "rcon_password testpass123")
	assert.Contains(t, contentStr, "gamemode0 test")
	assert.Contains(t, contentStr, "filterscripts admin")
}

func TestOpenMPConfigGeneration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "openmp_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create necessary directories for Open.MP
	err = os.MkdirAll(filepath.Join(tmpDir, "components"), 0o755)
	require.NoError(t, err)

	// Create an Open.MP runtime config
	cfg := &run.Runtime{
		WorkingDir: tmpDir,
		Platform:   "linux",
		Version:    "1.2.0-openmp",
		Mode:       run.Server,
	}

	// Set some basic config values
	hostname := "My Open.MP Test Server"
	port := 7777
	maxPlayers := 100
	rconPassword := "testpass123"

	cfg.Hostname = &hostname
	cfg.Port = &port
	cfg.MaxPlayers = &maxPlayers
	cfg.RCONPassword = &rconPassword
	cfg.Gamemodes = []string{"test"}
	cfg.Filterscripts = []string{"admin"}
	cfg.Plugins = []run.Plugin{"mysql", "streamer"}

	announce := true
	cfg.Announce = &announce

	// Test that it's detected as Open.MP
	assert.True(t, cfg.IsOpenMP())

	// Generate config
	err = GenerateConfig(cfg)
	require.NoError(t, err)

	// Check that config.json was created
	configJSONPath := filepath.Join(tmpDir, "config.json")
	assert.FileExists(t, configJSONPath)

	// Read the file and check the JSON structure
	content, err := os.ReadFile(configJSONPath)
	require.NoError(t, err)

	var config map[string]interface{}
	err = json.Unmarshal(content, &config)
	require.NoError(t, err)

	// Check basic fields
	assert.Equal(t, "My Open.MP Test Server", config["name"])
	assert.Equal(t, float64(100), config["max_players"])

	// Check network settings
	network, ok := config["network"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(7777), network["port"])

	// Check RCON settings
	rcon, ok := config["rcon"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "testpass123", rcon["password"])

	// Check Pawn settings
	pawn, ok := config["pawn"].(map[string]interface{})
	require.True(t, ok)

	mainScripts, ok := pawn["main_scripts"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, mainScripts, "test")

	sideScripts, ok := pawn["side_scripts"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, sideScripts, "admin")

	legacyPlugins, ok := pawn["legacy_plugins"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, legacyPlugins, "mysql")
	assert.Contains(t, legacyPlugins, "streamer")

	announceValue, ok := config["announce"].(bool)
	require.True(t, ok)
	assert.Equal(t, true, announceValue)
}

func TestConfigGeneratorSelection(t *testing.T) {
	// Test SA-MP generator selection
	sampCfg := &run.Runtime{
		WorkingDir: "/tmp",
		Version:    "0.3.7",
	}
	generator := GetConfigGenerator(sampCfg)
	assert.Equal(t, "server.cfg", generator.GetConfigFilename())

	// Test Open.MP generator selection
	openmpCfg := &run.Runtime{
		WorkingDir: "/tmp",
		Version:    "1.2.0-openmp",
	}
	generator = GetConfigGenerator(openmpCfg)
	assert.Equal(t, "config.json", generator.GetConfigFilename())
}

func TestExtraFieldsSupport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extra_fields_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an Open.MP runtime config with extra fields
	cfg := &run.Runtime{
		WorkingDir: tmpDir,
		Platform:   "linux",
		Version:    "1.2.0-openmp",
		Mode:       run.Server,
		Extra: map[string]string{
			"custom_plugin_setting": "custom_value",
			"another_setting":       "another_value",
		},
	}

	hostname := "Test Server"
	cfg.Hostname = &hostname

	// Generate config
	err = GenerateConfig(cfg)
	require.NoError(t, err)

	// Check that config.json was created
	configJSONPath := filepath.Join(tmpDir, "config.json")
	content, err := os.ReadFile(configJSONPath)
	require.NoError(t, err)

	var config map[string]interface{}
	err = json.Unmarshal(content, &config)
	require.NoError(t, err)

	// Check that extra fields are included
	assert.Equal(t, "custom_value", config["custom_plugin_setting"])
	assert.Equal(t, "another_value", config["another_setting"])
}

func TestOpenMPConfigGenerateWithExtraServerCfg(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "openmp_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &run.Runtime{
		WorkingDir:  tmpDir,
		RuntimeType: run.RuntimeTypeOpenMP,
		Extra: map[string]string{
			"legacy_setting": "legacy_value",
			"custom_config":  "123",
		},
	}

	// Generate the config
	generator := NewOpenMPConfig(tmpDir)
	err = generator.Generate(cfg)
	require.NoError(t, err)

	// Check that config.json was created
	configJSONPath := filepath.Join(tmpDir, "config.json")
	assert.FileExists(t, configJSONPath)

	// Check that server.cfg was created
	serverCfgPath := filepath.Join(tmpDir, "server.cfg")
	assert.FileExists(t, serverCfgPath)

	// Read and verify server.cfg contents
	content, err := os.ReadFile(serverCfgPath)
	require.NoError(t, err)

	contentStr := string(content)

	// Check for extra configuration values
	assert.Contains(t, contentStr, "legacy_setting legacy_value")
	assert.Contains(t, contentStr, "custom_config 123")
}

func TestOpenMPConfigGenerateWithoutExtraServerCfg(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "openmp_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &run.Runtime{
		WorkingDir:  tmpDir,
		RuntimeType: run.RuntimeTypeOpenMP,
		Extra:       map[string]string{},
	}

	// Generate the config
	generator := NewOpenMPConfig(tmpDir)
	err = generator.Generate(cfg)
	require.NoError(t, err)

	// Check that config.json was created
	configJSONPath := filepath.Join(tmpDir, "config.json")
	assert.FileExists(t, configJSONPath)

	// Check that server.cfg was NOT created
	serverCfgPath := filepath.Join(tmpDir, "server.cfg")
	assert.NoFileExists(t, serverCfgPath)
}
