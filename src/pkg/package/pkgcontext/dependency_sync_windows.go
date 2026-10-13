package pkgcontext

import (
	"errors"
	"os"
)

func syncDependencyRecoveryFile(file *os.File, info os.FileInfo) error {
	// FlushFileBuffers needs a write handle on Windows. These files are in
	// private staging; restore the read-only attribute before promoting them.
	mode := info.Mode().Perm()
	if err := os.Chmod(file.Name(), mode|0o200); err != nil {
		return err
	}
	writable, err := os.OpenFile(file.Name(), os.O_RDWR, 0)
	if err != nil {
		return errors.Join(err, os.Chmod(file.Name(), mode))
	}
	return errors.Join(writable.Sync(), writable.Close(), os.Chmod(file.Name(), mode))
}
