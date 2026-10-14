//go:build !windows

package fs

import (
	"errors"
	"os"
	"syscall"
)

// SyncDir flushes directory changes where the filesystem supports it.
func SyncDir(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}

	err = directory.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
		err = nil
	}

	return errors.Join(err, directory.Close())
}
