//go:build linux || darwin

package commands

import (
	"os"
	"syscall"
)

func checkCompilerSingleLink(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat != nil && stat.Nlink == 1
}
