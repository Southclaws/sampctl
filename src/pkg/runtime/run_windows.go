//go:build windows
// +build windows

package runtime

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/UserExistsError/conpty"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/sys/osversion"
)

func platformRun(_ context.Context, cmd *exec.Cmd, w io.Writer, r io.Reader, onStart func(*os.Process)) (err error) {
	version := osversion.Get()
	if version.Build >= osversion.V1809 {
		return usePty(cmd, w, r, onStart)
	}
	return useTty(cmd, w, r, onStart)
}

func usePty(cmd *exec.Cmd, w io.Writer, r io.Reader, onStart func(*os.Process)) (err error) {
	options := []conpty.ConPtyOption{}
	if cmd.Dir != "" {
		options = append(options, conpty.ConPtyWorkDir(cmd.Dir))
	}
	if len(cmd.Env) > 0 {
		options = append(options, conpty.ConPtyEnv(cmd.Env))
	}

	cpty, err := conpty.Start(buildConPtyCommandLine(cmd), options...)
	if err != nil {
		return errors.Wrap(err, "failed to start pty")
	}
	if cpty == nil {
		return errors.New("failed to create new pty, cpty is null")
	}

	if process, findErr := os.FindProcess(cpty.Pid()); findErr == nil {
		cmd.Process = process
		if onStart != nil {
			onStart(process)
		}
	}

	defer func() {
		if errClose := cpty.Close(); errClose != nil {
			if err == nil {
				err = errors.Wrap(errClose, "failed to close pty")
				return
			}
			print.Warn("failed to close pty:", errClose)
		}
	}()

	wrErrCh := make(chan error, 1)

	go func() {
		if r == nil {
			return
		}
		_, _ = io.Copy(cpty, r)
	}()
	go func() {
		_, errInner := io.Copy(w, cpty)
		wrErrCh <- errInner
	}()

	exitCode, waitErr := cpty.Wait(context.Background())
	if waitErr != nil {
		return errors.Wrap(waitErr, "failed to wait for pty process")
	}

	errWrite := <-wrErrCh
	if errWrite != nil && errWrite != io.EOF {
		print.Verb("write error", errWrite)
	}

	if exitCode == 0 || exitCode == 1 {
		return nil
	}

	return errors.Errorf("exit status %d", exitCode)
}

func buildConPtyCommandLine(cmd *exec.Cmd) string {
	args := append([]string(nil), cmd.Args...)
	if len(args) == 0 {
		args = []string{cmd.Path}
	} else if cmd.Path != "" {
		args[0] = cmd.Path
	}

	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, syscall.EscapeArg(arg))
	}

	return strings.Join(quoted, " ")
}

func useTty(cmd *exec.Cmd, w io.Writer, r io.Reader, onStart func(*os.Process)) (err error) {
	cmd.Stderr = w
	cmd.Stdout = w
	cmd.Stdin = r
	if err = cmd.Start(); err != nil {
		return err
	}
	if onStart != nil && cmd.Process != nil {
		onStart(cmd.Process)
	}
	err = cmd.Wait()
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

func terminateRuntimeProcess(process *os.Process) error {
	err := process.Kill()
	if err != nil && err.Error() == "process already finished" {
		return nil
	}
	return err
}
