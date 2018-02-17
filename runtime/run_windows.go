package runtime

// +build windows

import (
	"io"
	"os/exec"
)

func platformRun(cmd *exec.Cmd, w io.Writer) (err error) {
	cmd.Stdout = outputWriter
	return cmd.Run()
}
