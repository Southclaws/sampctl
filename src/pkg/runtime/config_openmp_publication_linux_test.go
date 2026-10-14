package runtime

import (
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestOpenMPConfigWriteFailurePreservesPreviousFiles(t *testing.T) {
	stage := os.Getenv("SAMPCTL_CONFIG_WRITE_FAILURE")
	if stage == "" {
		for _, name := range []string{"json", "legacy"} {
			t.Run(name, func(t *testing.T) {
				command := exec.Command(os.Args[0], "-test.run=^TestOpenMPConfigWriteFailurePreservesPreviousFiles$")
				command.Env = append(os.Environ(), "SAMPCTL_CONFIG_WRITE_FAILURE="+name)
				output, err := command.CombinedOutput()
				require.NoError(t, err, "%s", output)
			})
		}
		return
	}

	cfg := &run.Runtime{WorkingDir: t.TempDir(), RuntimeType: run.RuntimeTypeOpenMP}
	jsonPath := filepath.Join(cfg.WorkingDir, "config.json")
	legacyPath := filepath.Join(cfg.WorkingDir, "server.cfg")
	require.NoError(t, os.WriteFile(jsonPath, []byte(`{"previous":true}`), 0o600))
	require.NoError(t, os.WriteFile(legacyPath, []byte("previous legacy"), 0o600))
	limit := uint64(64)
	if stage == "legacy" {
		limit = 2000
		cfg.Extra = map[string]string{"network": strings.Repeat("x", 4096)}
		cfg.Network = map[string]any{"port": 7777}
	}
	signal.Ignore(syscall.SIGXFSZ)
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: limit, Max: limit}))

	err := GenerateConfig(cfg)
	content, readErr := os.ReadFile(jsonPath)
	require.NoError(t, readErr)
	if stage == "json" {
		require.Equal(t, `{"previous":true}`, string(content))
		require.ErrorContains(t, err, "failed to write config.json")
	} else {
		require.True(t, json.Valid(content))
		require.NotContains(t, string(content), "previous")
		require.ErrorContains(t, err, "failed to generate legacy server.cfg")
	}
	content, readErr = os.ReadFile(legacyPath)
	require.NoError(t, readErr)
	require.Equal(t, "previous legacy", string(content))
	require.ErrorIs(t, err, syscall.EFBIG)
	staged, globErr := filepath.Glob(filepath.Join(cfg.WorkingDir, ".*"))
	require.NoError(t, globErr)
	require.Empty(t, staged)
}
