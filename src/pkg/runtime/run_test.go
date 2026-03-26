package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestRunOptionsWithDefaults(t *testing.T) {
	t.Parallel()

	options := (RunOptions{}).withDefaults()
	require.NotNil(t, options.Output)

	buffer := &bytes.Buffer{}
	options = (RunOptions{Output: buffer}).withDefaults()
	assert.Same(t, buffer, options.Output)
}

func TestRunExecutesResolvedBinary(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	workingDir := t.TempDir()
	platform := currentTestPlatform()
	writeRuntimeFixtureManifest(t, cacheDir, "https://fixtures.example/linux", "https://fixtures.example/windows", "", "")

	binaryName := expectedRuntimeBinary(platform)
	binaryPath := filepath.Join(workingDir, binaryName)
	require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\nprintf 'run wrapper ok\\n'\n"), 0o755))

	var output bytes.Buffer
	err := Run(context.Background(), run.Runtime{
		WorkingDir: workingDir,
		Platform:   platform,
		Version:    "0.3.7",
		Mode:       run.Server,
	}, RunOptions{CacheDir: cacheDir, Output: &output})

	require.NoError(t, err)
	assert.Contains(t, output.String(), "run wrapper ok")
}

func TestWaitForRuntimeTermination(t *testing.T) {
	t.Parallel()

	t.Run("returns signal error", func(t *testing.T) {
		t.Parallel()

		sigCh := make(chan os.Signal, 1)
		sigCh <- syscall.SIGINT

		term := waitForRuntimeTermination(runtimeTerminationRequest{
			Context: context.Background(),
			Output:  io.Discard,
			SigCh:   sigCh,
		})

		require.Error(t, term.err)
		assert.Contains(t, term.err.Error(), "received signal: interrupt")
	})

	t.Run("returns termination after stream closes", func(t *testing.T) {
		t.Parallel()

		streamCh := make(chan string)
		close(streamCh)
		termCh := make(chan termination, 1)
		termCh <- termination{exit: true}

		term := waitForRuntimeTermination(runtimeTerminationRequest{
			Context:  context.Background(),
			Output:   io.Discard,
			StreamCh: streamCh,
			TermCh:   termCh,
		})

		assert.True(t, term.exit)
		assert.NoError(t, term.err)
	})

	t.Run("returns context error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		term := waitForRuntimeTermination(runtimeTerminationRequest{
			Context: ctx,
			Output:  io.Discard,
		})

		assert.ErrorIs(t, term.err, context.Canceled)
	})
}

func TestReadBinaryOutput(t *testing.T) {
	t.Parallel()

	t.Run("main only skips preamble and exits on end line", func(t *testing.T) {
		t.Parallel()

		reader, writer := io.Pipe()
		termCh := make(chan termination, 1)
		streamCh := make(chan string, 8)
		done := readBinaryOutput(outputReaderRequest{
			Context:      context.Background(),
			RunType:      run.MainOnly,
			OutputReader: reader,
			TermCh:       termCh,
			StreamCh:     streamCh,
		})

		go func() {
			defer writer.Close()
			_, _ = fmt.Fprintln(writer, "Loaded 1 filterscripts.")
			_, _ = fmt.Fprintln(writer)
			_, _ = fmt.Fprintln(writer, "hello world")
			_, _ = fmt.Fprintln(writer, "Number of vehicle models: 212")
		}()

		var lines []string
		for line := range streamCh {
			lines = append(lines, line)
		}
		<-done

		term := <-termCh
		assert.Equal(t, []string{"hello world"}, lines)
		assert.True(t, term.exit)
		assert.NoError(t, term.err)
	})

	t.Run("testing mode returns failure termination", func(t *testing.T) {
		t.Parallel()

		reader, writer := io.Pipe()
		termCh := make(chan termination, 1)
		streamCh := make(chan string, 4)
		done := readBinaryOutput(outputReaderRequest{
			Context:      context.Background(),
			RunType:      run.YTesting,
			OutputReader: reader,
			TermCh:       termCh,
			StreamCh:     streamCh,
		})

		go func() {
			defer writer.Close()
			_, _ = fmt.Fprintln(writer, "Loaded 1 filterscripts.")
			_, _ = fmt.Fprintln(writer)
			_, _ = fmt.Fprintln(writer, "*** Tests: 4, Fails: 1")
		}()

		for range streamCh {
		}
		<-done

		term := <-termCh
		require.Error(t, term.err)
		assert.EqualError(t, term.err, "tests failed")
		assert.True(t, term.exit)
	})

	t.Run("default mode emits lines until EOF", func(t *testing.T) {
		t.Parallel()

		reader, writer := io.Pipe()
		termCh := make(chan termination, 1)
		streamCh := make(chan string, 4)
		done := readBinaryOutput(outputReaderRequest{
			Context:      context.Background(),
			RunType:      run.Server,
			OutputReader: reader,
			TermCh:       termCh,
			StreamCh:     streamCh,
		})

		go func() {
			defer writer.Close()
			_, _ = fmt.Fprintln(writer, "first")
			_, _ = fmt.Fprintln(writer, "second")
		}()

		var lines []string
		for line := range streamCh {
			lines = append(lines, line)
		}
		<-done

		assert.Equal(t, []string{"first", "second"}, lines)
		select {
		case term := <-termCh:
			t.Fatalf("unexpected termination: %+v", term)
		default:
		}
	})
}

func TestProcessOutputLine(t *testing.T) {
	t.Parallel()

	t.Run("testing success exits cleanly", func(t *testing.T) {
		t.Parallel()

		state := &outputModeState{}
		line, emit, term, stop := processOutputLine(run.YTesting, state, "*** Tests: 4, Fails: 0")

		assert.Empty(t, line)
		assert.False(t, emit)
		require.NotNil(t, term)
		assert.True(t, term.exit)
		assert.NoError(t, term.err)
		assert.True(t, stop)
	})

	t.Run("server mode passes through", func(t *testing.T) {
		t.Parallel()

		state := &outputModeState{}
		line, emit, term, stop := processOutputLine(run.Server, state, "hello")

		assert.Equal(t, "hello", line)
		assert.True(t, emit)
		assert.Nil(t, term)
		assert.False(t, stop)
	})
}

func TestEvaluateRunResult(t *testing.T) {
	t.Parallel()

	t.Run("non recovering server wraps error", func(t *testing.T) {
		t.Parallel()

		term, next, retry := evaluateRunResult(runResultRequest{
			RunType: run.Server,
			Recover: false,
			Backoff: time.Second,
			RunErr:  errors.New("boom"),
		})

		require.Error(t, term.err)
		assert.Contains(t, term.err.Error(), "failed to start server")
		assert.Equal(t, time.Second, next)
		assert.False(t, retry)
	})

	t.Run("recovering server retries after short crash", func(t *testing.T) {
		t.Parallel()

		term, next, retry := evaluateRunResult(runResultRequest{
			RunType: run.Server,
			Recover: true,
			RunTime: 10 * time.Second,
			Backoff: time.Second,
			RunErr:  errors.New("boom"),
		})

		assert.NoError(t, term.err)
		assert.Equal(t, 2*time.Second, next)
		assert.True(t, retry)
	})

	t.Run("recovering server stops after repeated crashes", func(t *testing.T) {
		t.Parallel()

		term, next, retry := evaluateRunResult(runResultRequest{
			RunType: run.Server,
			Recover: true,
			RunTime: 10 * time.Second,
			Backoff: 8 * time.Second,
			RunErr:  errors.New("boom"),
		})

		require.Error(t, term.err)
		assert.Contains(t, term.err.Error(), "too many crashloops")
		assert.Equal(t, 16*time.Second, next)
		assert.False(t, retry)
	})

	t.Run("long runtime resets backoff", func(t *testing.T) {
		t.Parallel()

		_, next, retry := evaluateRunResult(runResultRequest{
			RunType: run.Server,
			Recover: true,
			RunTime: 2 * time.Minute,
			Backoff: 8 * time.Second,
			RunErr:  errors.New("boom"),
		})

		assert.Equal(t, time.Second, next)
		assert.True(t, retry)
	})
}

func TestSleepAndChannelHelpers(t *testing.T) {
	t.Parallel()

	t.Run("sleep stops on cancel", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.False(t, sleepWithContext(ctx, time.Second))
	})

	t.Run("send termination stops on cancel", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.False(t, sendTermination(ctx, make(chan termination), termination{}))
	})

	t.Run("send output stops on cancel", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.False(t, sendOutputLine(ctx, make(chan string), "line"))
	})
}

func TestCommandTracker(t *testing.T) {
	t.Parallel()

	tracker := &commandTracker{}
	assert.Nil(t, tracker.current())

	process := &os.Process{Pid: 42}
	tracker.set(process)
	assert.Same(t, process, tracker.current())
}

func TestWrapRuntimeError(t *testing.T) {
	t.Parallel()

	assert.NoError(t, wrapRuntimeError(nil))
	require.EqualError(t, wrapRuntimeError(errors.New("boom")), "received runtime error: boom")
}

func TestCreateSpecialLink(t *testing.T) {
	t.Parallel()

	t.Run("creates missing symlink", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		cfg := run.Runtime{WorkingDir: workingDir, RootLink: true}

		require.NoError(t, createSpecialLink(cfg))

		target, err := os.Readlink(filepath.Join(workingDir, "scriptfiles", "DANGEROUS_SERVER_ROOT"))
		require.NoError(t, err)
		assert.Equal(t, workingDir, target)
	})

	t.Run("replaces regular file with symlink", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		scriptfilesDir := filepath.Join(workingDir, "scriptfiles")
		require.NoError(t, os.MkdirAll(scriptfilesDir, 0o755))
		specialLink := filepath.Join(scriptfilesDir, "DANGEROUS_SERVER_ROOT")
		require.NoError(t, os.WriteFile(specialLink, []byte("not-a-link"), 0o644))

		require.NoError(t, createSpecialLink(run.Runtime{WorkingDir: workingDir, RootLink: true}))

		info, err := os.Lstat(specialLink)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink)
	})

	t.Run("removes existing symlink when root link disabled", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		scriptfilesDir := filepath.Join(workingDir, "scriptfiles")
		require.NoError(t, os.MkdirAll(scriptfilesDir, 0o755))
		specialLink := filepath.Join(scriptfilesDir, "DANGEROUS_SERVER_ROOT")
		require.NoError(t, os.Symlink(workingDir, specialLink))

		require.NoError(t, createSpecialLink(run.Runtime{WorkingDir: workingDir, RootLink: false}))
		_, err := os.Lstat(specialLink)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestTestResultsFromLine(t *testing.T) {
	t.Parallel()

	results := testResultsFromLine("*** Tests: 12, Fails: 3")
	assert.Equal(t, 12, results.Tests)
	assert.Equal(t, 3, results.Fails)
}

func TestExecuteRuntime(t *testing.T) {
	t.Run("successful execution streams output", func(t *testing.T) {
		t.Parallel()

		scriptPath := filepath.Join(t.TempDir(), "runtime-success.sh")
		require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf 'hello runtime\\n'\n"), 0o755))

		var output bytes.Buffer
		err := executeRuntime(context.Background(), runtimeExecution{
			binary:  scriptPath,
			runType: run.Server,
			output:  &output,
		})

		require.NoError(t, err)
		assert.Contains(t, output.String(), "hello runtime")
	})

	t.Run("context cancellation returns wrapped error", func(t *testing.T) {
		t.Parallel()

		scriptPath := filepath.Join(t.TempDir(), "runtime-sleep.sh")
		require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nwhile :; do sleep 1; done\n"), 0o755))

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := executeRuntime(ctx, runtimeExecution{
			binary:  scriptPath,
			runType: run.Server,
			output:  io.Discard,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "received runtime error")
	})
}
