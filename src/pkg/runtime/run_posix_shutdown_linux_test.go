package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

const shutdownHelperRole = "SAMPCTL_PTY_SHUTDOWN_ROLE"

type descendantShutdownOutput struct {
	safeBuffer
	leader         chan int
	descendant     chan int
	leaderOnce     sync.Once
	descendantOnce sync.Once
}

func (output *descendantShutdownOutput) Write(data []byte) (int, error) {
	n, err := output.safeBuffer.Write(data)
	lines := strings.Split(output.String(), "\n")
	for _, line := range lines[:len(lines)-1] {
		var pid int
		if count, _ := fmt.Sscanf(strings.TrimSpace(line), "leader:%d", &pid); count == 1 {
			output.leaderOnce.Do(func() { output.leader <- pid })
		}
		if count, _ := fmt.Sscanf(strings.TrimSpace(line), "descendant:%d", &pid); count == 1 {
			output.descendantOnce.Do(func() { output.descendant <- pid })
		}
	}
	return n, err
}

func TestPlatformRunDescendantShutdown(t *testing.T) {
	if scenario := os.Getenv("SAMPCTL_PTY_SHUTDOWN_CASE"); scenario != "" {
		// Adopt and reap the fixture's orphaned descendant in this isolated process.
		require.NoError(t, unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0))
		runDescendantShutdownCase(t, scenario)
		return
	}

	for _, scenario := range []string{"normal", "exit status", "cancel after exit", "cancel while running", "execute cancellation"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPlatformRunDescendantShutdown$")
			cmd.Env = append(os.Environ(), "SAMPCTL_PTY_SHUTDOWN_CASE="+scenario)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
		})
	}
}

func runDescendantShutdownCase(t *testing.T, scenario string) {
	t.Helper()

	dir := t.TempDir()
	exitPath := filepath.Join(dir, "leader-control")
	releasePath := filepath.Join(dir, "descendant-control")
	for _, path := range []string{exitPath, releasePath} {
		require.NoError(t, unix.Mkfifo(path, 0o600))
	}
	exitControl, err := os.OpenFile(exitPath, os.O_RDWR, 0)
	require.NoError(t, err)
	releaseControl, err := os.OpenFile(releasePath, os.O_RDWR, 0)
	require.NoError(t, err)
	t.Setenv(shutdownHelperRole, "leader")
	t.Setenv("SAMPCTL_PTY_LEADER_CONTROL", exitPath)
	t.Setenv("SAMPCTL_PTY_DESCENDANT_CONTROL", releasePath)
	t.Setenv("SAMPCTL_PTY_TEST_BINARY", os.Args[0])
	if scenario == "exit status" {
		t.Setenv("SAMPCTL_PTY_LEADER_STATUS", "7")
	}

	input, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	flags := runtimeInputFileFlags(t, input)
	var source io.Reader = input
	if scenario == "normal" {
		source = nil
	}
	output := &descendantShutdownOutput{leader: make(chan int, 1), descendant: make(chan int, 1)}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan error, 1)
	var descendant *os.Process
	finished := false
	t.Cleanup(func() {
		cancel()
		_ = exitControl.Close()
		_ = releaseControl.Close()
		if descendant != nil {
			if killErr := descendant.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				t.Error(killErr)
			}
		}
		if !finished {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("runtime did not return during fixture cleanup")
			}
		}
		if descendant != nil {
			_, waitErr := descendant.Wait()
			if waitErr != nil {
				t.Error(waitErr)
			}
		}
	})

	if scenario == "execute cancellation" {
		binary := filepath.Join(dir, "runtime")
		script := "#!/bin/sh\nexec \"$SAMPCTL_PTY_TEST_BINARY\" -test.run=^TestRuntimePtyShutdownHelper$\n"
		require.NoError(t, os.WriteFile(binary, []byte(script), 0o700))
		go func() {
			done <- executeRuntime(ctx, runtimeExecution{binary: binary, runType: run.Server, output: output, input: source})
		}()
	} else {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimePtyShutdownHelper$")
		go func() { done <- platformRun(ctx, cmd, output, source, nil) }()
	}

	leaderPID := waitShutdownProcess(t, output.leader)
	descendantPID := waitShutdownProcess(t, output.descendant)
	descendant, err = os.FindProcess(descendantPID)
	require.NoError(t, err)
	if scenario != "cancel while running" {
		_, err = io.WriteString(exitControl, "exit\n")
		require.NoError(t, err)
		waitShutdownLeaderReaped(t, ctx, leaderPID)
	}
	if scenario == "normal" || scenario == "exit status" {
		_, err = io.WriteString(releaseControl, "o")
		require.NoError(t, err)
	}
	if strings.Contains(scenario, "cancel") {
		cancel()
	}

	select {
	case err = <-done:
		finished = true
	case <-time.After(5 * time.Second):
		t.Fatal("runtime remained blocked on the descendant-held pty")
	}
	switch scenario {
	case "execute cancellation":
		require.ErrorIs(t, err, context.Canceled)
	case "exit status":
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError)
		require.Equal(t, 7, exitError.ExitCode())
	case "cancel while running":
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError)
	default:
		require.NoError(t, err)
	}
	if scenario == "normal" || scenario == "exit status" {
		require.NoError(t, descendant.Signal(syscall.Signal(0)), "normal exit must not forcibly kill the descendant")
		require.Equal(t, 1024, strings.Count(output.String(), "final output"))
		require.Contains(t, output.String(), "descendant output")
	}
	_, err = io.WriteString(releaseControl, "probe\n")
	require.NoError(t, err)
	var state *os.ProcessState
	waitDone := make(chan error, 1)
	go func(process *os.Process) {
		var waitErr error
		state, waitErr = process.Wait()
		waitDone <- waitErr
	}(descendant)
	select {
	case err = <-waitDone:
	case <-time.After(5 * time.Second):
		if killErr := descendant.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			t.Error(killErr)
		}
		<-waitDone
		descendant = nil
		t.Fatal("descendant did not finish checking pty closure")
	}
	descendant = nil
	require.NoError(t, err)
	require.True(t, state.Success(), "descendant must observe closure of the pty master")

	require.Equal(t, flags, runtimeInputFileFlags(t, input))
	_, err = io.WriteString(writer, "caller input\n")
	require.NoError(t, err)
	require.NoError(t, input.SetReadDeadline(time.Now().Add(time.Second)))
	data := make([]byte, len("caller input\n"))
	_, err = io.ReadFull(input, data)
	require.NoError(t, err)
	require.Equal(t, "caller input\n", string(data))
}

func waitShutdownProcess(t *testing.T, ready <-chan int) int {
	t.Helper()
	select {
	case pid := <-ready:
		return pid
	case <-time.After(5 * time.Second):
		t.Fatal("runtime fixture did not become ready")
		return 0
	}
}

func waitShutdownLeaderReaped(t *testing.T, ctx context.Context, pid int) {
	t.Helper()
	for {
		var status unix.Siginfo
		err := unix.Waitid(unix.P_PID, pid, &status, unix.WEXITED|unix.WNOWAIT|unix.WNOHANG, nil)
		if errors.Is(err, syscall.ECHILD) {
			return
		}
		if !errors.Is(err, syscall.EINTR) {
			require.NoError(t, err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("runtime leader was not reaped")
		default:
			goruntime.Gosched()
		}
	}
}

func TestRuntimePtyShutdownHelper(t *testing.T) {
	role := os.Getenv(shutdownHelperRole)
	if role == "" {
		return
	}
	if role == "descendant" {
		signal.Ignore(syscall.SIGHUP)
		control, err := os.Open(os.Getenv("SAMPCTL_PTY_DESCENDANT_CONTROL"))
		require.NoError(t, err)
		fmt.Printf("descendant:%d\n", os.Getpid())
		for {
			command := make([]byte, 1)
			n, readErr := control.Read(command)
			if readErr != nil {
				require.ErrorIs(t, readErr, io.EOF)
				break
			}
			if n == 0 {
				continue
			}
			if command[0] == 'o' {
				fmt.Println("descendant output")
				continue
			}
			poll := []unix.PollFd{{Fd: int32(os.Stdin.Fd()), Events: unix.POLLIN}}
			_, err = unix.Poll(poll, 0)
			require.NoError(t, err)
			require.NotZero(t, poll[0].Revents&unix.POLLHUP)
			break
		}
		require.NoError(t, control.Close())
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimePtyShutdownHelper$")
	cmd.Env = append(os.Environ(), shutdownHelperRole+"=descendant")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	require.NoError(t, cmd.Start())
	fmt.Printf("leader:%d\n", os.Getpid())
	control, err := os.Open(os.Getenv("SAMPCTL_PTY_LEADER_CONTROL"))
	require.NoError(t, err)
	_, err = control.Read(make([]byte, 1))
	require.NoError(t, err)
	require.NoError(t, control.Close())
	if scenario := os.Getenv("SAMPCTL_PTY_SHUTDOWN_CASE"); scenario == "normal" || scenario == "exit status" {
		fmt.Print(strings.Repeat("final output\n", 1024))
	}
	if os.Getenv("SAMPCTL_PTY_LEADER_STATUS") == "7" {
		os.Exit(7)
	}
	os.Exit(0)
}
