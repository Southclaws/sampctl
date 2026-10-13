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
	return fmt.Errorf("%s", NotInstalledMessage())
}

func NotInstalledMessage() string {
	return "git was not found in PATH. Install it or add it to PATH:\n" +
		"  winget install --id git.git -e --source winget\n" +
		"  scoop install git\n" +
		"  https://git-scm.com"
}
