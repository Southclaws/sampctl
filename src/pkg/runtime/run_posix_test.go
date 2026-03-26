//go:build !windows

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
	"github.com/stretchr/testify/require"
)

const (
	runtimeSignalHelperEnv  = "SAMPCTL_RUNTIME_SIGNAL_HELPER"
	runtimeSignalReadyLine  = "runtime-ready"
	runtimeSignalScriptName = "runtime-signal.sh"
)

type blockingReader struct {
	release <-chan struct{}
}

type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (buffer *safeBuffer) Write(p []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.b.Write(p)
}

func (buffer *safeBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.b.String()
}

func (reader blockingReader) Read(_ []byte) (int, error) {
	<-reader.release
	return 0, io.EOF
}

func TestPlatformRunFallbackCopiesIO(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-c", "cat")
	input := bytes.NewBufferString("hello from stdin")
	var output bytes.Buffer

	err := platformRunFallback(cmd, &output, input)
	require.NoError(t, err)
	require.Equal(t, "hello from stdin", output.String())
}

func TestGetRuntimeSysProcAttr(t *testing.T) {
	t.Parallel()

	attr := getRuntimeSysProcAttr()
	require.NotNil(t, attr)
	require.True(t, attr.Setpgid)
	require.Equal(t, syscall.SIGTERM, attr.Pdeathsig)
}

func TestGetRuntimePtySysProcAttr(t *testing.T) {
	t.Parallel()

	attr := getRuntimePtySysProcAttr()
	require.NotNil(t, attr)
	require.Equal(t, syscall.SIGTERM, attr.Pdeathsig)
}

func TestTerminateRuntimeProcess(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	err := terminateRuntimeProcess(cmd.Process)
	require.NoError(t, err)

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case waitErr := <-waitDone:
		require.Error(t, waitErr)
	case <-time.After(2 * time.Second):
		t.Fatal("process was not terminated")
	}
}

func TestPlatformRunReturnsWhenProcessExitsWithBlockingInput(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	defer close(release)

	done := make(chan error, 1)
	go func() {
		cmd := exec.Command("/bin/sh", "-c", "exit 0") //nolint:gosec
		done <- platformRun(cmd, io.Discard, blockingReader{release: release})
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("platformRun did not return after child process exit")
	}
}

func TestExecuteRuntimeHandlesSIGINTEndToEnd(t *testing.T) {
	stdout, stderr, cmd := startRuntimeSignalHelper(t)
	waitForRuntimeReady(t, stdout, stderr)

	require.NoError(t, cmd.Process.Signal(syscall.SIGINT))
	require.NoError(t, waitForHelperExit(cmd, stderr))
}

func TestExecuteRuntimeHandlesSIGINTHelper(t *testing.T) {
	if os.Getenv(runtimeSignalHelperEnv) != "1" {
		return
	}

	scriptPath := writeRuntimeSignalScript(t)
	err := executeRuntime(context.Background(), runtimeExecution{
		binary:  scriptPath,
		runType: run.Server,
		output:  os.Stdout,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "received signal:")
}

func startRuntimeSignalHelper(t *testing.T) (*bufio.Scanner, *safeBuffer, *exec.Cmd) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestExecuteRuntimeHandlesSIGINTHelper$")
	cmd.Env = append(os.Environ(), runtimeSignalHelperEnv+"=1")

	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)

	stderr := &safeBuffer{}
	cmd.Stderr = stderr

	require.NoError(t, cmd.Start())

	return bufio.NewScanner(stdoutPipe), stderr, cmd
}

func waitForRuntimeReady(t *testing.T, stdout *bufio.Scanner, stderr *safeBuffer) {
	t.Helper()

	readyCh := make(chan error, 1)
	go func() {
		for stdout.Scan() {
			if strings.TrimSpace(stdout.Text()) == runtimeSignalReadyLine {
				readyCh <- nil
				return
			}
		}
		readyCh <- stdout.Err()
	}()

	select {
	case err := <-readyCh:
		require.NoErrorf(t, err, "helper failed before runtime became ready: %s", stderr.String())
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for runtime readiness; stderr: %s", stderr.String())
	}
}

func waitForHelperExit(cmd *exec.Cmd, stderr *safeBuffer) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err == nil {
			return nil
		}
		return fmt.Errorf("helper process failed: %w: %s", err, stderr.String())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for helper exit: %s", stderr.String())
	}
}

func writeRuntimeSignalScript(t *testing.T) string {
	t.Helper()

	scriptPath := filepath.Join(t.TempDir(), runtimeSignalScriptName)
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '" + runtimeSignalReadyLine + "'\n" +
		"while :; do\n" +
		"  sleep 1\n" +
		"done\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	return scriptPath
}
