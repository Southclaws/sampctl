// +build !windows

package runtime

import (
	"io"
	"os/exec"
	"sync"

	"github.com/kr/pty"
	"github.com/pkg/errors"
)

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return
	}

	defer ptmx.Close()

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
		if errInner.Error() == "read /dev/ptmx: input/output error" {
			errInner = errors.New("server crashed")
		}
		rdErrCh <- errInner
		wg.Done()
	}()

	wg.Wait()

	errRead := <-rdErrCh
	errWrite := <-wrErrCh

	if errRead != nil || errWrite != nil {
		err = errors.Errorf("read error: '%s' write error: '%s'", errRead, errWrite)
	}

	return
}
