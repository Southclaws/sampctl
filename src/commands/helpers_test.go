package commands

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestGetCommandState(t *testing.T) {
	t.Parallel()

	_, err := getCommandState(nil)
	require.EqualError(t, err, "command context is not available")

	ctx := cli.NewContext(cli.NewApp(), flag.NewFlagSet("test", flag.ContinueOnError), nil)
	_, err = getCommandState(ctx)
	require.EqualError(t, err, "command state is not available")

	state := newCommandState("1.2.3", t.TempDir())
	app := cli.NewApp()
	app.Metadata = map[string]interface{}{commandStateKey: state}
	ctx = cli.NewContext(app, flag.NewFlagSet("test", flag.ContinueOnError), nil)

	got, err := getCommandState(ctx)
	require.NoError(t, err)
	assert.Same(t, state, got)
}

func TestGetCommandConfig(t *testing.T) {
	t.Parallel()

	app := cli.NewApp()
	state := newCommandState("1.2.3", t.TempDir())
	app.Metadata = map[string]interface{}{commandStateKey: state}
	ctx := cli.NewContext(app, flag.NewFlagSet("test", flag.ContinueOnError), nil)

	_, err := getCommandConfig(ctx)
	require.EqualError(t, err, "config is not available")

	state.cfg = &config.Config{DefaultUser: "tester"}
	got, err := getCommandConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "tester", got.DefaultUser)
}

func TestGetCommandEnv(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	app := cli.NewApp()
	flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
	flagSet.Bool("verbose", false, "")
	flagSet.String("platform", "", "")
	require.NoError(t, flagSet.Parse([]string{"--verbose", "--platform=linux"}))
	ctx := cli.NewContext(app, flagSet, nil)

	env, err := getCommandEnv(ctx)
	require.NoError(t, err)
	expectedCacheDir, err := fs.ConfigDir()
	require.NoError(t, err)
	assert.True(t, env.Verbose)
	assert.Equal(t, "linux", env.Platform)
	assert.Equal(t, expectedCacheDir, env.CacheDir)
}

func TestNewGitHubClient(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, newGitHubClient(""))
	assert.NotNil(t, newGitHubClient("token"))
}

func TestGenerateDocsIncludesCommandsAndFlags(t *testing.T) {
	t.Parallel()

	state := newCommandState("1.2.3", t.TempDir())
	app := newCLIApp("1.2.3", state)
	app.Author = "Southclaws"
	app.Email = "hello@southcla.ws"
	app.Description = "Test description"

	docs := GenerateDocs(app)

	assert.Contains(t, docs, "# `sampctl`")
	assert.Contains(t, docs, "## Commands")
	assert.Contains(t, docs, "### `sampctl init`")
	assert.Contains(t, docs, "#### Subcommands")
	assert.Contains(t, docs, "## Global Flags")
	assert.Contains(t, docs, "verbose")
	assert.Contains(t, docs, "platform")
	assert.Contains(t, docs, "bare")
	assert.Contains(t, docs, "Usage: `sampctl build [build name]`")
}
