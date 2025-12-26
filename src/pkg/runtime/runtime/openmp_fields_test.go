package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestPawnYAMLRuntimeOpenMPFieldsArePreserved(t *testing.T) {
	data := []byte(`preset: openmp
runtime:
  discord:
    invite: https://discord.gg/example
  rcon_config:
    allow_teleport: true
`)

	var pkg pawnpackage.Package
	require.NoError(t, yaml.Unmarshal(data, &pkg))

	cfg, err := pkg.GetRuntimeConfig("")
	require.NoError(t, err)
	require.True(t, cfg.IsOpenMP())

	tmpDir, err := os.MkdirTemp("", "pawnpackage-openmp-fields")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg.WorkingDir = tmpDir
	cfg.Platform = "linux"

	require.NoError(t, GenerateConfig(&cfg))

	content, err := os.ReadFile(filepath.Join(tmpDir, "config.json"))
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &parsed))

	discord, ok := parsed["discord"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "https://discord.gg/example", discord["invite"])

	rcon, ok := parsed["rcon"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, rcon["allow_teleport"])
}
