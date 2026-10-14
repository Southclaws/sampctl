//go:build !windows

package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type runtimeInputOutput struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (output *runtimeInputOutput) Write(data []byte) (int, error) {
	n, err := output.buffer.Write(data)
	if strings.Contains(output.buffer.String(), "input-ready") {
		output.once.Do(func() { close(output.ready) })
	}
	return n, err
}

func (output *runtimeInputOutput) String() string {
	return output.buffer.String()
}

func TestPlatformRunLeavesCallerInputAvailableAfterExit(t *testing.T) {
	t.Parallel()

	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	flags := runtimeInputFileFlags(t, input)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, done := startRuntimeInputCommand(t, ctx, input)
	_, err = io.WriteString(writer, "first command\n")
	require.NoError(t, err)
	require.NoError(t, waitRuntimeInputCommand(t, done))
	require.Contains(t, output.String(), "received:first command")
	require.Equal(t, flags, runtimeInputFileFlags(t, input))

	const next = "input for the caller\n"
	_, err = io.WriteString(writer, next)
	require.NoError(t, err)
	require.NoError(t, input.SetReadDeadline(time.Now().Add(time.Second)))
	got := make([]byte, len(next))
	_, err = io.ReadFull(input, got)
	require.NoError(t, err, "a completed runtime must not consume later caller input")
	require.Equal(t, next, string(got))
}

func TestPlatformRunReusesCallerInputAcrossExecutions(t *testing.T) {
	t.Parallel()

	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	flags := runtimeInputFileFlags(t, input)

	for attempt := range 8 {
		command := fmt.Sprintf("command %d", attempt)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		output, done := startRuntimeInputCommand(t, ctx, input)
		_, err = io.WriteString(writer, command+"\n")
		require.NoError(t, err)
		err = waitRuntimeInputCommand(t, done)
		cancel()
		require.NoError(t, err, "the current execution must receive %q", command)
		require.Contains(t, output.String(), "received:"+command)
		require.Equal(t, flags, runtimeInputFileFlags(t, input))
	}
}

func TestPlatformRunLeavesCallerTerminalAvailableAfterExit(t *testing.T) {
	t.Parallel()

	terminal, input, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = terminal.Close() })
	// Fd puts the fixture in blocking mode, as terminal stdin can be.
	_ = input.Fd()
	flags := runtimeInputFileFlags(t, input)
	require.Zero(t, flags&unix.O_NONBLOCK)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, done := startRuntimeInputCommand(t, ctx, input)
	_, err = io.WriteString(terminal, "terminal command\n")
	require.NoError(t, err)
	require.NoError(t, waitRuntimeInputCommand(t, done))
	require.Contains(t, output.String(), "received:terminal command")
	require.Equal(t, flags, runtimeInputFileFlags(t, input))

	const next = "terminal input for the caller\n"
	_, err = io.WriteString(terminal, next)
	require.NoError(t, err)
	got := make([]byte, len(next))
	raw, err := input.SyscallConn()
	require.NoError(t, err)
	var readErr, restoreErr error
	var n int
	err = raw.Control(func(fd uintptr) {
		readErr = unix.SetNonblock(int(fd), true)
		if readErr != nil {
			return
		}
		defer func() { restoreErr = unix.SetNonblock(int(fd), false) }()

		var ready int
		ready, readErr = unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 1000)
		if readErr != nil {
			return
		}
		if ready == 0 {
			readErr = os.ErrDeadlineExceeded
			return
		}

		n, readErr = unix.Read(int(fd), got)
	})
	require.NoError(t, err)
	require.NoError(t, restoreErr)
	require.NoError(t, readErr, "a completed runtime must not consume later terminal input")
	require.Equal(t, next, string(got[:n]))
	require.Equal(t, flags, runtimeInputFileFlags(t, input))
}

func TestPlatformRunCancellationWhileTerminalInputIsFull(t *testing.T) {
	t.Parallel()

	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	flags := runtimeInputFileFlags(t, input)
	_, err = io.WriteString(writer, "initial command\n")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output := &runtimeInputOutput{ready: make(chan struct{})}
	done := make(chan error, 1)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", //nolint:gosec
		"stty -echo -icanon min 1 time 0; IFS= read -r command; printf 'input-ready\\n'; kill -STOP $$")
	go func() {
		done <- platformRun(ctx, cmd, output, input, nil)
	}()
	select {
	case <-output.ready:
	case err = <-done:
		t.Fatalf("runtime exited before input readiness: %v", err)
	case <-ctx.Done():
		t.Fatal("runtime did not acknowledge its initial input")
	}

	data := bytes.Repeat([]byte{'x'}, 4<<20)
	require.NoError(t, writer.SetWriteDeadline(time.Now().Add(time.Second)))
	n, err := writer.Write(data)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	require.Positive(t, n)
	require.Less(t, n, len(data))
	cancel()

	require.Error(t, waitRuntimeInputCommand(t, done))
	require.Equal(t, flags, runtimeInputFileFlags(t, input))
	_, err = input.Stat()
	require.NoError(t, err, "cancellation must not close caller-owned input")
}

func TestPlatformRunPreservesCallerInputReadDeadline(t *testing.T) {
	t.Parallel()

	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	flags := runtimeInputFileFlags(t, input)
	require.NoError(t, input.SetReadDeadline(time.Now().Add(-time.Second)))

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output := &runtimeInputOutput{ready: make(chan struct{})}
	done := make(chan error, 1)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeInputReadDeadlineHelper$")
	cmd.Env = append(os.Environ(), "SAMPCTL_RUNTIME_INPUT_READ_DEADLINE_HELPER=1")
	go func() {
		done <- platformRun(ctx, cmd, output, input, nil)
	}()
	select {
	case <-output.ready:
	case err = <-done:
		t.Fatalf("runtime exited before input readiness: %v", err)
	case <-ctx.Done():
		t.Fatal("runtime did not become ready for input")
	}

	const next = "input after the read deadline\n"
	_, err = io.WriteString(writer, next)
	require.NoError(t, err)
	require.NoError(t, waitRuntimeInputCommand(t, done))
	require.Contains(t, output.String(), "input-idle")
	require.Equal(t, flags, runtimeInputFileFlags(t, input))
	_, err = input.Read(make([]byte, 1))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)

	require.NoError(t, input.SetReadDeadline(time.Now().Add(time.Second)))
	got := make([]byte, len(next))
	_, err = io.ReadFull(input, got)
	require.NoError(t, err)
	require.Equal(t, next, string(got))
}

func TestRuntimeInputReadDeadlineHelper(t *testing.T) {
	if os.Getenv("SAMPCTL_RUNTIME_INPUT_READ_DEADLINE_HELPER") != "1" {
		return
	}

	fmt.Println("input-ready")
	ready, err := unix.Poll([]unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}, 500)
	require.NoError(t, err)
	require.Zero(t, ready, "input with an expired caller deadline must not reach the child")
	fmt.Println("input-idle")
}

func TestPlatformRunCancellationLeavesCallerInputOpen(t *testing.T) {
	t.Parallel()

	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	flags := runtimeInputFileFlags(t, input)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, done := startRuntimeInputCommand(t, ctx, input)
	cancel()
	require.Error(t, waitRuntimeInputCommand(t, done))
	require.Equal(t, flags, runtimeInputFileFlags(t, input))

	const next = "input after cancellation\n"
	_, err = io.WriteString(writer, next)
	require.NoError(t, err)
	require.NoError(t, input.SetReadDeadline(time.Now().Add(time.Second)))
	got := make([]byte, len(next))
	_, err = io.ReadFull(input, got)
	require.NoError(t, err, "a cancelled runtime must leave caller input usable")
	require.Equal(t, next, string(got))
}

func startRuntimeInputCommand(t *testing.T, ctx context.Context, input *os.File) (*runtimeInputOutput, <-chan error) {
	t.Helper()

	output := &runtimeInputOutput{ready: make(chan struct{})}
	done := make(chan error, 1)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", //nolint:gosec
		"printf 'input-ready\\n'; IFS= read -r command; printf 'received:%s\\n' \"$command\"")
	go func() {
		done <- platformRun(ctx, cmd, output, input, nil)
	}()

	select {
	case <-output.ready:
	case err := <-done:
		t.Fatalf("runtime exited before input readiness: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not become ready for input")
	}

	return output, done
}

func waitRuntimeInputCommand(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
		return nil
	}
}

func runtimeInputFileFlags(t *testing.T, input *os.File) int {
	t.Helper()

	raw, err := input.SyscallConn()
	require.NoError(t, err)
	var flags int
	var flagsErr error
	err = raw.Control(func(fd uintptr) {
		flags, flagsErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
	})
	require.NoError(t, err)
	require.NoError(t, flagsErr)

	return flags
}
