package resource

import (
	"os"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func ensureCacheDir(cacheDirPath string) error {
	if err := fs.EnsureDir(cacheDirPath, fs.PermDirPrivate); err != nil {
		// Backward compatibility: older cache layout may have stored a file
		// where we now expect a directory.
		if info, statErr := os.Stat(cacheDirPath); statErr == nil && !info.IsDir() {
			_ = os.Remove(cacheDirPath)
			if retryErr := fs.EnsureDir(cacheDirPath, fs.PermDirPrivate); retryErr == nil {
				return nil
			}
		}
		return errors.Wrap(err, "failed to create cache directory")
	}
	return nil
}
