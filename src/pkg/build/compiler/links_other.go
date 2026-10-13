//go:build !linux && !darwin && !windows

package compiler

import "os"

func compilerSingleLink(os.FileInfo) bool { return true }

func compilerPathSingleLink(_ string, info os.FileInfo) (bool, error) {
	return compilerSingleLink(info), nil
}
