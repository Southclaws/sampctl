// +build windows

package runtime

import (
	"io"
	"os/exec"
)

func platformRun(cmd *exec.Cmd, w io.Writer) (err error) {
	cmd.Stdout = w
	return cmd.Run()
}
