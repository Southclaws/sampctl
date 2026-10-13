//go:build linux || darwin

package compiler

import (
	"os"
	"syscall"
)

func compilerSingleLink(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat != nil && stat.Nlink == 1
}

func compilerPathSingleLink(_ string, info os.FileInfo) (bool, error) {
	return compilerSingleLink(info), nil
}
