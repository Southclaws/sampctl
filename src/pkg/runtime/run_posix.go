//go:build !windows
// +build !windows

package runtime

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func platformRun(ctx context.Context, cmd *exec.Cmd, w io.Writer, r io.Reader, onStart func(*os.Process)) (err error) {
	cmd.SysProcAttr = getRuntimePtySysProcAttr()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		if cmd.Process != nil {
			return errors.Wrap(err, "failed to start command with pty")
		}
		print.Verb("PTY allocation failed, falling back to regular execution:", err)
		return platformRunFallback(cloneRuntimeCommand(ctx, cmd), w, r, onStart)
	}
	if ptmx == nil {
		return errors.New("failed to create new pty, ptmx is null")
	}
	if onStart != nil && cmd.Process != nil {
		onStart(cmd.Process)
	}

	var closeOnce sync.Once
	closePty := func() error {
		var closeErr error
		closeOnce.Do(func() {
			closeErr = ptmx.Close()
		})
		return closeErr
	}

	defer func() {
		if errClose := closePty(); errClose != nil {
			if isBenignPtyCopyError(errClose) {
				return
			}
			if err == nil {
				err = errors.Wrap(errClose, "failed to close pty")
				return
			}
			print.Warn("failed to close pty:", errClose)
		}
	}()

	cmdErrCh := make(chan error, 1)
	wrErrCh := make(chan error, 1)

	go func() {
		if r == nil {
			return
		}
		_, errInner := io.Copy(ptmx, r)
		if errInner != nil && !isBenignPtyCopyError(errInner) {
			print.Verb("read error", errInner)
		}
	}()
	go func() {
		_, errInner := io.Copy(w, ptmx)
		wrErrCh <- errInner
	}()
	go func() {
		cmdErrCh <- cmd.Wait()
	}()

	cmdErr := <-cmdErrCh
	errWrite := <-wrErrCh
	if errWrite != nil && !isBenignPtyCopyError(errWrite) {
		print.Verb("write error", errWrite)
	}
	if errClose := closePty(); errClose != nil && !isBenignPtyCopyError(errClose) {
		print.Verb("failed to close pty after command exit:", errClose)
	}

	return cmdErr
}

func cloneRuntimeCommand(ctx context.Context, cmd *exec.Cmd) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}

	path := cmd.Path
	if path == "" && len(cmd.Args) > 0 {
		path = cmd.Args[0]
	}

	args := make([]string, 0, max(0, len(cmd.Args)-1))
	if len(cmd.Args) > 1 {
		args = append(args, cmd.Args[1:]...)
	}

	cloned := exec.CommandContext(ctx, path, args...) //nolint:gosec
	cloned.Dir = cmd.Dir
	if len(cmd.Env) > 0 {
		cloned.Env = append([]string(nil), cmd.Env...)
	}

	return cloned
}

func platformRunFallback(cmd *exec.Cmd, w io.Writer, r io.Reader, onStart func(*os.Process)) error {
	cmd.SysProcAttr = getRuntimeSysProcAttr()
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Stdin = r

	err := cmd.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start command")
	}
	if onStart != nil && cmd.Process != nil {
		onStart(cmd.Process)
	}

	return cmd.Wait()
}

func getRuntimeSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}

func getRuntimePtySysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}
}

func terminateRuntimeProcess(process *os.Process) error {
	pgid, err := syscall.Getpgid(process.Pid)
	if err == nil {
		if err = syscall.Kill(-pgid, syscall.SIGKILL); err == nil || stderrors.Is(err, syscall.ESRCH) {
			return nil
		}
	}

	err = process.Kill()
	if err == nil || stderrors.Is(err, os.ErrProcessDone) {
		return nil
	}

	return err
}

func isBenignPtyCopyError(err error) bool {
	return stderrors.Is(err, os.ErrClosed) || stderrors.Is(err, syscall.EIO)
}
