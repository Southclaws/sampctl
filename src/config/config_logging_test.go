package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func TestLoadConfigVerboseOmitsCredentials(t *testing.T) {
	if path := os.Getenv("SAMPCTL_TEST_CONFIG_OUTPUT_PATH"); path != "" {
		print.SetVerbose()
		_, err := loadConfigFile(path)
		require.NoError(t, err)
		return
	}
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"github_token":"fixture-secret-token","git_password":"fixture-secret-password"}`), 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestLoadConfigVerboseOmitsCredentials$")
	command.Env = append(os.Environ(), "SAMPCTL_TEST_CONFIG_OUTPUT_PATH="+path)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.NotContains(t, string(output), "fixture-secret-token")
	assert.NotContains(t, string(output), "fixture-secret-password")
	assert.Contains(t, string(output), path)
}
