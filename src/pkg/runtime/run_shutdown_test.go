//go:build !windows

package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type runtimeOutputWriter func([]byte) (int, error)

func (write runtimeOutputWriter) Write(data []byte) (int, error) {
	return write(data)
}

func TestExecuteRuntimeCancelsWhileStreamingOutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output := runtimeOutputWriter(func(data []byte) (int, error) {
		cancel()
		return len(data), nil
	})

	err := executeRuntimeWithTimeout(t, ctx, run.Server, output, "while :; do printf '%02000d\\n' 1; done\n")
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunBinaryDoesNotReportCancellationAsStartupFailure(t *testing.T) {
	t.Parallel()

	filename := filepath.Join(t.TempDir(), "server.sh")
	require.NoError(t, os.WriteFile(filename, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0o755))

	// A cancelled context and a writable result channel were previously both
	// eligible when the runner reported the killed command's exit status.
	for attempt := 0; attempt < 32; attempt++ {
		ctx, cancel := context.WithCancel(t.Context())
		termCh := make(chan termination, 1)
		starts := 0
		runBinary(ctx, binaryRunConfig{
			binary:       filename,
			runType:      run.Server,
			outputWriter: io.Discard,
			termCh:       termCh,
			onStart: func(*os.Process) {
				starts++
				cancel()
			},
		})
		cancel()

		require.Equal(t, 1, starts, "attempt %d must cancel an actual child process", attempt)
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		select {
		case term := <-termCh:
			t.Fatalf("attempt %d reported cancelled command as runtime termination: %v", attempt, term.err)
		default:
		}
	}
}

func TestExecuteRuntimePreservesFinalUnterminatedLine(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := executeRuntimeWithTimeout(t, t.Context(), run.Server, &output, "printf 'first\\nfinal'\n")
	require.NoError(t, err)
	require.Equal(t, "first\nfinal\n", output.String())
}

func TestExecuteRuntimeStopsAfterOutputFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("output unavailable")
	output := runtimeOutputWriter(func([]byte) (int, error) { return 0, want })

	err := executeRuntimeWithTimeout(t, t.Context(), run.Server, output, "while :; do printf '%02000d\\n' 1; done\n")
	require.ErrorIs(t, err, want)
}

func TestExecuteRuntimeStopsAfterCompletionWithFurtherOutput(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		mode   run.RunMode
		marker string
	}{
		{run.MainOnly, "Number of vehicle models: 212"},
		{run.YTesting, "*** Tests: 4, Fails: 0"},
	} {
		t.Run(string(test.mode), func(t *testing.T) {
			t.Parallel()

			script := "printf 'Loaded 1 filterscripts.\\n\\n" + test.marker + "\\n'\n" +
				"while :; do printf '%02000d\\n' 1; done\n"
			require.NoError(t, executeRuntimeWithTimeout(t, t.Context(), test.mode, io.Discard, script))
		})
	}
}

func TestExecuteRuntimeReturnsOutputReadFailure(t *testing.T) {
	t.Parallel()

	err := executeRuntimeWithTimeout(t, t.Context(), run.Server, io.Discard,
		"printf '%070000d\\n' 1\nwhile :; do printf 'more output\\n'; done\n")
	require.ErrorContains(t, err, "failed to read runtime output")
}

func executeRuntimeWithTimeout(t *testing.T, parent context.Context, mode run.RunMode, output io.Writer, script string) error {
	t.Helper()

	filename := filepath.Join(t.TempDir(), "server.sh")
	require.NoError(t, os.WriteFile(filename, []byte("#!/bin/sh\n"+script), 0o755))
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- executeRuntime(ctx, runtimeExecution{binary: filename, runType: mode, output: output})
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not finish after output termination")
		return nil
	}
}
