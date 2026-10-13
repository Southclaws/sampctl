package commands

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
)

func TestPackageInitReportsInitializationFailure(t *testing.T) {
	t.Cleanup(configdir.Refresh)
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()

	ctx := newTestCLIContext("dev")
	state, err := getCommandState(ctx)
	if err != nil {
		t.Fatalf("getCommandState() = %v, want command state", err)
	}
	state.cfg = &config.Config{DefaultUser: "tester"}
	missingDir := filepath.Join(t.TempDir(), "missing")
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.String("dir", missingDir, "")
	set.String("preset", "openmp", "")
	set.Bool("verbose", true, "")
	ctx = cli.NewContext(ctx.App, set, nil)
	var output bytes.Buffer
	ctx.App.Writer = &output

	err = packageInit(ctx)
	if err == nil {
		t.Fatalf("packageInit(dir=%q) = nil, want initialization failure", missingDir)
	}
	var reported *commandExitError
	if !errors.As(err, &reported) || !reported.reported || ExitCode(err) != 1 || err.Error() != "" {
		t.Errorf("packageInit(dir=%q) = %v with exit code %d, want reported failure and no duplicate message", missingDir, err, ExitCode(err))
	}
	text := output.String()
	if !strings.Contains(text, "Dependency setup incomplete") || !strings.Contains(text, "Details: directory does not exist") {
		t.Errorf("packageInit(dir=%q) output = %q, want setup context and verbose directory cause", missingDir, text)
	}
	if strings.Contains(text, "credentials") {
		t.Errorf("packageInit(dir=%q) output = %q, want no credential advice", missingDir, text)
	}
}

func TestValidateInitDirectoryAllowsEmptyDirectory(t *testing.T) {
	t.Parallel()

	err := validateInitDirectory(t.TempDir())
	require.NoError(t, err)
}

func TestValidateInitDirectoryRejectsExistingPackage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(`{"runtime":{"version":"0.3.7"}}`), 0o644))

	err := validateInitDirectory(dir)
	require.Error(t, err)
	assert.EqualError(t, err, "Directory already appears to be a package")
}

func TestValidateInitDirectoryRejectsInvalidPackageDefinitionState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(`{"runtime":{"version":"0.3.7"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.yaml"), []byte("runtime:\n  version: 0.3.7\n"), 0o644))

	err := validateInitDirectory(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect package definition")
}
