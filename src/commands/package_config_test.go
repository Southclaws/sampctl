package commands

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestPackageConfigRequiresValue(t *testing.T) {
	ctx, _ := newPackageConfigTestContext(t, []string{"DefaultUser"})

	err := packageConfig(ctx)
	require.EqualError(t, err, "missing value for config field DefaultUser")
}

func TestPackageConfigRejectsInvalidField(t *testing.T) {
	ctx, _ := newPackageConfigTestContext(t, []string{"MissingField", "value"})

	err := packageConfig(ctx)
	require.EqualError(t, err, "unknown config field MissingField. Run 'sampctl config' to list fields")
}

func TestPackageConfigSetsStringFieldAndWritesConfig(t *testing.T) {
	ctx, state := newPackageConfigTestContext(t, []string{"DefaultUser", "bob"})

	err := packageConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "bob", state.cfg.DefaultUser)

	cacheDir, err := fs.ConfigDir()
	require.NoError(t, err)

	contents, err := os.ReadFile(filepath.Join(cacheDir, "config.json"))
	require.NoError(t, err)
	assert.Contains(t, string(contents), `"default_user": "bob"`)
}

func TestPackageConfigSetsBoolPointerField(t *testing.T) {
	ctx, state := newPackageConfigTestContext(t, []string{"HideVersionUpdateMessage", "true"})

	err := packageConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, state.cfg.HideVersionUpdateMessage)
	assert.True(t, *state.cfg.HideVersionUpdateMessage)
}

func TestPackageConfigRejectsInvalidBoolPointerValue(t *testing.T) {
	ctx, _ := newPackageConfigTestContext(t, []string{"HideVersionUpdateMessage", "banana"})

	err := packageConfig(ctx)
	require.EqualError(t, err, "config field HideVersionUpdateMessage requires true or false")
}

func TestDisplayConfigPrintsFields(t *testing.T) {
	falseValue := false
	cfg := &config.Config{
		DefaultUser:              "alice",
		GitHubToken:              "token",
		HideVersionUpdateMessage: &falseValue,
	}

	output := captureStdout(t, func() {
		displayConfig(reflect.ValueOf(cfg).Elem())
	})

	assert.Contains(t, output, "DefaultUser")
	assert.Contains(t, output, "alice")
	assert.Contains(t, output, "GitHubToken")
	assert.Contains(t, output, "token")
	assert.Contains(t, output, "HideVersionUpdateMessage")
	assert.Contains(t, output, "false")
}

func newPackageConfigTestContext(t *testing.T, args []string) (*cli.Context, *commandState) {
	t.Helper()

	t.Cleanup(configdir.Refresh)
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()

	cacheDir := fs.ConfigDirPath()
	if rel, err := filepath.Rel(configHome, cacheDir); err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("ConfigDirPath() = %q, want path inside temporary home %q (relative path %q, error %v)", cacheDir, configHome, rel, err)
	}

	falseValue := false
	state := newCommandState("1.2.3", configHome)
	state.cfg = &config.Config{
		DefaultUser:              "alice",
		GitHubToken:              "token",
		HideVersionUpdateMessage: &falseValue,
	}

	app := cli.NewApp()
	app.Metadata = map[string]interface{}{commandStateKey: state}

	flagSet := flag.NewFlagSet("config", flag.ContinueOnError)
	flagSet.Bool("verbose", false, "")
	flagSet.String("platform", "", "")
	require.NoError(t, flagSet.Parse(args))

	return cli.NewContext(app, flagSet, nil), state
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()
	require.NoError(t, writer.Close())

	contents, err := io.ReadAll(reader)
	require.NoError(t, err)

	return string(contents)
}
