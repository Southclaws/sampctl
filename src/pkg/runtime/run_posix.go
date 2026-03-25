//go:build !windows
// +build !windows

package runtime

import (
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

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	cmd.SysProcAttr = getRuntimePtySysProcAttr()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		print.Verb("PTY allocation failed, falling back to regular execution:", err)
		return platformRunFallback(cmd, w, r)
	}
	if ptmx == nil {
		return errors.New("failed to create new pty, ptmx is null")
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
	if errClose := closePty(); errClose != nil && !isBenignPtyCopyError(errClose) {
		print.Verb("failed to close pty after command exit:", errClose)
	}

	errWrite := <-wrErrCh
	if errWrite != nil && !isBenignPtyCopyError(errWrite) {
		print.Verb("write error", errWrite)
	}

	return cmdErr
}

func platformRunFallback(cmd *exec.Cmd, w io.Writer, r io.Reader) error {
	cmd.SysProcAttr = getRuntimeSysProcAttr()
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Stdin = r

	err := cmd.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start command")
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
