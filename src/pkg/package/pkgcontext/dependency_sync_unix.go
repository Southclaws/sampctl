//go:build !windows

package pkgcontext

import "os"

func syncDependencyRecoveryFile(file *os.File, _ os.FileInfo) error {
	return file.Sync()
}
