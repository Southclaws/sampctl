//go:build !windows
// +build !windows

package runtime

import (
	"io"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		print.Verb("PTY allocation failed, falling back to regular execution:", err)
		return platformRunFallback(cmd, w, r)
	}
	if ptmx == nil {
		return errors.New("failed to create new pty, ptmx is null")
	}

	defer func() {
		errDefer := ptmx.Close()
		if errDefer != nil {
			panic(errDefer)
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	rdErrCh := make(chan error, 1)
	wrErrCh := make(chan error, 1)

	go func() {
		_, errInner := io.Copy(ptmx, r)
		rdErrCh <- errInner
		wg.Done()
	}()
	go func() {
		_, errInner := io.Copy(w, ptmx)
		wrErrCh <- errInner
		wg.Done()
	}()

	wg.Wait()

	errRead := <-rdErrCh
	if errRead != nil {
		print.Verb("read error", errRead)
	}
	errWrite := <-wrErrCh
	if errWrite != nil {
		print.Verb("write error", errWrite)
	}

	return nil
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
