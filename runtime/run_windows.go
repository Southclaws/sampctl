//go:build windows
// +build windows

package runtime

import (
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"rs3.io/go/mserr/ntstatus"
)

func platformRun(cmd *exec.Cmd, w io.Writer, r io.Reader) (err error) {
	cmd.Stderr = w
	cmd.Stdout = w
	cmd.Stdin = r
	err = cmd.Run()
	// process kill on windows: "exit status 1"
	if err != nil {
		if err.Error() == "exit status 1" {
			err = nil
			return
		}

		if strings.Contains(err.Error(), "exit status") {
			statusCodeStr := strings.Split(err.Error(), " ")[2]
			statusCodeInt, innerError := strconv.ParseInt(statusCodeStr, 0, 64)
			if innerError != nil {
				return innerError
			}
			statusCode := ntstatus.NTStatus(uint32(statusCodeInt))
			err = errors.Errorf("exit status %s\n", statusCode.String())
		}
	}

	return
}
