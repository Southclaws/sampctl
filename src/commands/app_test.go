package commands

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestRunBareVersion(t *testing.T) {
	t.Cleanup(configdir.Refresh)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	configdir.Refresh()

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
		"check",
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

func TestIsCheckInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "check", args: []string{"sampctl", "check"}, want: true},
		{name: "global flags", args: []string{"sampctl", "--verbose", "--platform", "linux", "check", "--fix"}, want: true},
		{name: "global flag named check", args: []string{"sampctl", "--platform", "check", "version"}},
		{name: "command argument", args: []string{"sampctl", "get", "check"}},
		{name: "help command", args: []string{"sampctl", "help", "check"}, want: true},
		{name: "other help command", args: []string{"sampctl", "help", "version", "check"}},
		{name: "global help", args: []string{"sampctl", "--help", "check"}, want: true},
		{name: "separator", args: []string{"sampctl", "--", "check"}, want: true},
		{name: "invalid global flag", args: []string{"sampctl", "--unknown", "check"}, want: true},
		{name: "invalid global flag value", args: []string{"sampctl", "--verbose=invalid", "check"}, want: true},
		{name: "missing executable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isCheckInvocation(tt.args))
		})
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", want: 0},
		{name: "existing command error", err: errors.New("failed"), want: 1},
		{name: "health failure", err: &commandExitError{code: 1}, want: 1},
		{name: "internal error", err: &commandExitError{code: 2, err: errors.New("failed")}, want: 2},
		{name: "wrapped internal error", err: fmt.Errorf("check: %w", &commandExitError{code: 2, err: errors.New("failed")}), want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExitCode(tt.err))
		})
	}
}

func TestCommandExitErrorPreservesCauseWithoutCLIExitCoder(t *testing.T) {
	t.Parallel()

	cause := errors.New("fixture failure")
	err := &commandExitError{code: 2, err: cause}

	assert.ErrorIs(t, err, cause)
	assert.Equal(t, cause.Error(), err.Error())
	assert.Empty(t, (&commandExitError{code: 1}).Error())

	reported := &commandExitError{code: 2, err: cause, reported: true}
	assert.Empty(t, reported.Error())
	assert.ErrorIs(t, reported, cause)

	_, exits := any(err).(cli.ExitCoder)
	assert.False(t, exits, "urfave must not exit from library code")
}

func TestRunCheckHelpDoesNotCreateManagedState(t *testing.T) {
	t.Cleanup(configdir.Refresh)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	configdir.Refresh()

	for _, args := range [][]string{
		{"sampctl", "--verbose", "check", "--help"},
		{"sampctl", "help", "check"},
	} {
		require.NoError(t, Run(args, "1.2.3"))
		assert.NoDirExists(t, fs.ConfigDirPath())
	}
}

func TestRunCheckPreservesLegacyStateAndClassifiesUsageError(t *testing.T) {
	t.Cleanup(configdir.Refresh)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	configdir.Refresh()

	legacyDir := filepath.Join(os.Getenv("HOME"), ".samp")
	require.NoError(t, os.Mkdir(legacyDir, 0o700))
	legacyConfig := filepath.Join(legacyDir, "config.json")
	contents := []byte(`{"default_user":"legacy"}`)
	require.NoError(t, os.WriteFile(legacyConfig, contents, 0o600))

	for _, args := range [][]string{
		{"sampctl", "check", "--unknown-check-option"},
		{"sampctl", "--verbose=invalid", "check"},
		{"sampctl", "--unknown", "check"},
	} {
		err := Run(args, "1.2.3")
		require.Error(t, err)
		assert.Equal(t, 2, ExitCode(err))
		assert.NoDirExists(t, fs.ConfigDirPath())

		got, err := os.ReadFile(legacyConfig)
		require.NoError(t, err)
		assert.Equal(t, contents, got)
	}
}

func TestCheckAppSkipsConfigLifecycle(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	configPath := filepath.Join(cacheDir, "config.json")
	contents := []byte("{broken")
	require.NoError(t, os.WriteFile(configPath, contents, 0o600))

	state := newCommandState("1.2.3", cacheDir)
	app := newCLIApp("1.2.3", state)
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	var ran bool
	for i := range app.Commands {
		if app.Commands[i].Name == "check" {
			app.Commands[i].Action = func(*cli.Context) error {
				ran = true
				return nil
			}
		}
	}

	require.NoError(t, app.Run([]string{"sampctl", "check"}))
	assert.True(t, ran)
	assert.Nil(t, state.cfg)
	assert.Nil(t, state.gh)
	assert.Nil(t, state.gitAuth)

	got, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, contents, got)
}

func TestCheckCommandExitCodesAndOutput(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		contents   string
		args       []string
		code       int
		diagnostic string
	}{
		{name: "outside project", diagnostic: "– Skipped: No project configuration found"},
		{name: "unhealthy project", contents: "{broken", code: 1, diagnostic: "✗ Project configuration is malformed"},
		{name: "invalid arguments", args: []string{"unexpected"}, code: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.contents != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(tc.contents), 0o600))
			}

			cacheDir := filepath.Join(t.TempDir(), "missing")
			state := newCommandState("1.2.3", cacheDir)
			app := newCLIApp("1.2.3", state)
			var output bytes.Buffer
			app.Writer = &output
			app.ErrWriter = &output
			args := append([]string{"sampctl", "check", "--dir", dir}, tc.args...)

			err := app.Run(args)
			assert.Equal(t, tc.code, ExitCode(err), output.String())
			assert.NoDirExists(t, cacheDir)

			if tc.diagnostic != "" {
				assert.Equal(t, 1, strings.Count(output.String(), tc.diagnostic), output.String())
			}

			if tc.code == 1 {
				require.Error(t, err)
				assert.Empty(t, err.Error(), "an already rendered health failure must not be printed again")
				assert.NotContains(t, output.String(), "invalid character")
			}

			if tc.code == 2 {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "does not accept positional arguments")
				assert.Empty(t, output.String())
			}
		})
	}
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

func TestPlatformResolvesGlobalAndCommandFlags(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "global", args: []string{"--platform", "windows", "build"}, want: "windows"},
		{name: "command", args: []string{"build", "--platform", "darwin"}, want: "darwin"},
		{name: "command overrides global", args: []string{"--platform", "windows", "build", "--platform", "darwin"}, want: "darwin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newCLIApp("test", newCommandState("test", t.TempDir()))
			app.Before, app.After = nil, nil
			var selected string
			for i := range app.Commands {
				if app.Commands[i].Name == "build" {
					app.Commands[i].Action = func(c *cli.Context) error {
						selected = platform(c)
						return nil
					}
				}
			}

			require.NoError(t, app.Run(append([]string{"sampctl"}, tc.args...)))
			assert.Equal(t, tc.want, selected)
		})
	}
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
