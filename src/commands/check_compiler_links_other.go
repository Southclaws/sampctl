//go:build !linux && !darwin

package commands

import "os"

func checkCompilerSingleLink(os.FileInfo) bool {
	return false
}
