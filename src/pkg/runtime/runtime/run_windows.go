//go:build windows
// +build windows

package runtime

import (
	"io"
	"os/exec"
	"sync"
	"syscall"

	"github.com/UserExistsError/conpty"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/sys/osversion"
)

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	version := osversion.Get()
	if version.Build >= osversion.V1809 {
		return usePty(cmd, w, r)
	}
	return useTty(cmd, w, r)
}

func usePty(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	cpty, err := conpty.Start(cmd.Path)
	if err != nil {
		return errors.Wrap(err, "failed to start pty")
	}
	if cpty == nil {
		return errors.New("failed to create new pty, cpty is null")
	}

	defer func() {
		errDefer := cpty.Close()
		if errDefer != nil {
			panic(errDefer)
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	rdErrCh := make(chan error, 1)
	wrErrCh := make(chan error, 1)

	go func() {
		_, errInner := io.Copy(cpty, r)
		rdErrCh <- errInner
		wg.Done()
	}()
	go func() {
		_, errInner := io.Copy(w, cpty)
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

	return
}

func useTty(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	cmd.Stderr = w
	cmd.Stdout = w
	cmd.Stdin = r
	err = cmd.Run()
	// process kill on windows: "exit status 1"
	if err != nil && err.Error() == "exit status 1" {
		err = nil
	}
	return
}

func getRuntimeSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}
