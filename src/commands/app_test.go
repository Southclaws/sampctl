package commands

import (
	"context"
	"flag"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"
)

func TestRunBareVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	require.NoError(t, Run([]string{"sampctl", "--bare", "version"}, "1.2.3"))
}

func TestNewCLIAppConfiguresMetadataAndCommands(t *testing.T) {
	t.Parallel()

	state := newCommandState("1.2.3", t.TempDir())
	app := newCLIApp("1.2.3", state)

	require.NotNil(t, app)
	assert.Equal(t, "sampctl", app.Name)
	assert.Equal(t, "1.2.3", app.Version)
	assert.True(t, app.EnableBashCompletion)
	assert.Same(t, state, app.Metadata[commandStateKey])

	commandNames := make([]string, 0, len(app.Commands))
	for _, cmd := range app.Commands {
		commandNames = append(commandNames, cmd.Name)
	}

	assert.Equal(t, []string{
		"init",
		"ensure",
		"install",
		"uninstall",
		"release",
		"config",
		"get",
		"build",
		"run",
		"compiler",
		"template",
		"version",
		"completion",
		"docs",
	}, commandNames)
	assert.Len(t, app.Flags, len(globalFlags()))
	assert.NotNil(t, app.Before)
	assert.NotNil(t, app.After)
	assert.NotNil(t, app.OnUsageError)
}

func TestWithGlobalFlagsAppendsLocalFlags(t *testing.T) {
	t.Parallel()

	global := []cli.Flag{
		cli.BoolFlag{Name: "verbose"},
		cli.StringFlag{Name: "platform"},
	}
	local := []cli.Flag{
		cli.BoolFlag{Name: "watch"},
	}

	flags := withGlobalFlags(global, local)
	require.Len(t, flags, 3)
	assert.Equal(t, "verbose", flags[0].GetName())
	assert.Equal(t, "platform", flags[1].GetName())
	assert.Equal(t, "watch", flags[2].GetName())
}

func TestPlatformUsesFlagOrRuntimeDefault(t *testing.T) {
	t.Parallel()

	app := cli.NewApp()
	flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
	flagSet.String("platform", "", "")
	require.NoError(t, flagSet.Parse([]string{"--platform=linux"}))
	ctx := cli.NewContext(app, flagSet, nil)
	assert.Equal(t, "linux", platform(ctx))

	defaultSet := flag.NewFlagSet("test-default", flag.ContinueOnError)
	defaultSet.String("platform", "", "")
	ctx = cli.NewContext(app, defaultSet, nil)
	assert.Equal(t, runtime.GOOS, platform(ctx))
}

func TestNewCommandContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := newCommandContext()
	cancel()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("context was not canceled")
	}
}

func TestNewCommandTimeoutContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := newCommandTimeoutContext(50 * time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(50*time.Millisecond), deadline, 200*time.Millisecond)

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed context did not expire")
	}
}
