// +build windows

package runtime

import (
	"io"
	"os/exec"
)

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
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
