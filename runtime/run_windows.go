// +build windows

package runtime

import (
	"io"
	"os/exec"
)

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	cmd.Stdout = w
	cmd.Stdin = r
	return cmd.Run()
}
