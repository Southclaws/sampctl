package fs

import (
	"os"
	"path/filepath"
)

func EnsureDir(dir string, perm os.FileMode) error {
	return os.MkdirAll(dir, perm)
}

func EnsureDirForFile(path string, perm os.FileMode) error {
	return EnsureDir(filepath.Dir(path), perm)
}
