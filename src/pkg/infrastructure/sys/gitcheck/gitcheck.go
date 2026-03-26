package gitcheck

import (
	"fmt"
	"os/exec"
)

func IsInstalled() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func RequireInstalled() error {
	if IsInstalled() {
		return nil
	}
	return fmt.Errorf(
		"git is not installed on your system.\n" +
			"sampctl requires Git to manage dependencies.\n" +
			"please install Git from https://git-scm.com and try again",
	)
}
