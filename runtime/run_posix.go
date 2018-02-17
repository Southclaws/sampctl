package runtime

// +build linux, darwin

import (
	"io"
	"os/exec"

	"github.com/kr/pty"
	"github.com/pkg/errors"
)

func platformRun(cmd *exec.Cmd, w io.Writer) (err error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return
	}

	defer ptmx.Close()
	_, err = io.Copy(w, ptmx)
	if err.Error() == "read /dev/ptmx: input/output error" {
		err = errors.New("server crashed")
	}
	return
}
