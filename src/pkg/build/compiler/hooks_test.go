package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
)

func TestBuildHookContracts(t *testing.T) {
	hooks := []struct {
		stage string
		run   func(context.Context, build.Config, io.Writer) error
	}{
		{stage: "pre-build", run: RunPreBuildCommands},
		{stage: "post-build", run: RunPostBuildCommands},
	}

	for _, hook := range hooks {
		t.Run(hook.stage, func(t *testing.T) {
			workingDir := t.TempDir()
			run := func(ctx context.Context, commands [][]string, output io.Writer) error {
				cfg := build.Config{WorkingDir: workingDir}
				if hook.stage == "pre-build" {
					cfg.PreBuildCommands = commands
				} else {
					cfg.PostBuildCommands = commands
				}
				return hook.run(ctx, cfg, output)
			}

			t.Run("preserves arguments output order and inherited process state", func(t *testing.T) {
				t.Setenv("SAMPCTL_BUILD_HOOK_VALUE", "inherited value")
				cwd, err := os.Getwd()
				require.NoError(t, err)
				arguments := []string{"first argument", "", "$SAMPCTL_BUILD_HOOK_VALUE", "semi;colon", `"quoted"`}
				commands := [][]string{
					buildHookCommand("inspect", arguments...),
					buildHookCommand("emit", "second-out", "second-err"),
				}

				var output bytes.Buffer
				require.NoError(t, run(context.Background(), commands, &output))
				encoded, remaining, ok := strings.Cut(output.String(), "\n")
				require.True(t, ok, "missing process observation: %q", output.String())

				var observation buildHookObservation
				require.NoError(t, json.Unmarshal([]byte(encoded), &observation))
				assert.Equal(t, arguments, observation.Args)
				assert.Equal(t, cwd, observation.Dir)
				assert.Equal(t, "inherited value", observation.Env)
				assert.Equal(t, "stderr\nsecond-outsecond-err", remaining)
			})

			t.Run("returns the raw exit error and stops following commands", func(t *testing.T) {
				commands := [][]string{
					buildHookCommand("exit"),
					buildHookCommand("emit", "unexpected", "unexpected"),
				}
				var output bytes.Buffer
				err := run(context.Background(), commands, &output)
				require.IsType(t, &exec.ExitError{}, err)
				assert.Equal(t, 3, err.(*exec.ExitError).ExitCode())
				assert.Equal(t, "before-failure", output.String())
			})

			t.Run("rejects empty entries before checking cancellation", func(t *testing.T) {
				entries := []struct {
					name    string
					command []string
				}{
					{name: "nil"},
					{name: "empty", command: []string{}},
				}
				for _, entry := range entries {
					for _, canceled := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/canceled=%t", entry.name, canceled), func(t *testing.T) {
							ctx, cancel := context.WithCancel(context.Background())
							defer cancel()
							if canceled {
								cancel()
							}

							var output bytes.Buffer
							commands := [][]string{entry.command, buildHookCommand("emit", "unexpected", "")}
							err := run(ctx, commands, &output)
							require.EqualError(t, err, hook.stage+" command is empty")
							assert.Empty(t, output.String())
						})
					}
				}
			})

			t.Run("preserves pre-canceled and empty-list results", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				var output bytes.Buffer
				err := run(ctx, [][]string{buildHookCommand("emit", "unexpected", "")}, &output)
				require.Same(t, context.Canceled, err)
				assert.Empty(t, output.String())
				require.NoError(t, run(ctx, nil, &output))
				require.NoError(t, run(ctx, [][]string{}, &output))
			})

			t.Run("returns the output writer error directly", func(t *testing.T) {
				writeErr := errors.New("hook output failed")
				output := &buildHookWriter{write: func([]byte) (int, error) { return 0, writeErr }}
				err := run(context.Background(), [][]string{buildHookCommand("emit", "output", "")}, output)
				require.Same(t, writeErr, err)
			})

			t.Run("preserves the raw process error after active cancellation", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				var output bytes.Buffer
				writer := &buildHookWriter{write: func(data []byte) (int, error) {
					n, err := output.Write(data)
					if strings.Contains(output.String(), "ready\n") {
						cancel()
					}
					return n, err
				}}
				commands := [][]string{
					buildHookCommand("wait"),
					buildHookCommand("emit", "unexpected", ""),
				}

				err := run(ctx, commands, writer)
				require.Same(t, context.Canceled, ctx.Err())
				require.IsType(t, &exec.ExitError{}, err)
				assert.NotErrorIs(t, err, context.Canceled)
				assert.Equal(t, "ready\n", output.String())
			})
		})
	}
}

type buildHookObservation struct {
	Args []string
	Dir  string
	Env  string
}

type buildHookWriter struct {
	write func([]byte) (int, error)
}

func (w *buildHookWriter) Write(data []byte) (int, error) {
	return w.write(data)
}

func buildHookCommand(mode string, arguments ...string) []string {
	command := []string{os.Args[0], "-test.run=^TestBuildHookProcessHelper$", "--", "sampctl-build-hook", mode}
	return append(command, arguments...)
}

func TestBuildHookProcessHelper(t *testing.T) {
	separator := slices.Index(os.Args, "--")
	if separator == -1 || len(os.Args) <= separator+2 || os.Args[separator+1] != "sampctl-build-hook" {
		return
	}
	arguments := os.Args[separator+3:]

	switch os.Args[separator+2] {
	case "inspect":
		cwd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(os.Stdout).Encode(buildHookObservation{
			Args: arguments,
			Dir:  cwd,
			Env:  os.Getenv("SAMPCTL_BUILD_HOOK_VALUE"),
		}))
		fmt.Fprint(os.Stderr, "stderr\n")
	case "emit":
		fmt.Fprint(os.Stdout, arguments[0])
		fmt.Fprint(os.Stderr, arguments[1])
	case "exit":
		fmt.Fprint(os.Stdout, "before-failure")
		os.Exit(3)
	case "wait":
		fmt.Fprint(os.Stdout, "ready\n")
		for {
			time.Sleep(time.Hour)
		}
	default:
		t.Fatalf("unknown build hook helper mode: %s", os.Args[separator+2])
	}

	os.Exit(0)
}
