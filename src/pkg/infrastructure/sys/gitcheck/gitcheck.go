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
			"sampctl requires git to manage dependencies.\n" +
			"install git using one of these methods:\n" +
			"  1. winget install --id git.git -e --source winget\n" +
			"  2. scoop install git\n" +
			"  3. download from https://git-scm.com",
	)
}
