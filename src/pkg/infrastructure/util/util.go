package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies src to dst, trying a hard link before copying the contents.
// If both paths refer to the same file, it returns success.
func CopyFile(src, dst string) (err error) {
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return
	}

	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sourceInfo.Name(), sourceInfo.Mode().String())
	}

	destinationInfo, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !destinationInfo.Mode().IsRegular() {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", destinationInfo.Name(), destinationInfo.Mode().String())
		}

		if os.SameFile(sourceInfo, destinationInfo) {
			return
		}
	}

	if err = os.Link(src, dst); err == nil {
		return
	}

	return copyFileContents(src, dst)
}

// copyFileContents creates or replaces dst with the contents of src.
func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close() // nolint

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return
	}

	return out.Sync()
}

// FullPath returns an absolute path when possible, otherwise it falls back to the input path.
func FullPath(dir string) string {
	path, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return path
}

// RelPath makes a path relative to the cwd, if it fails it simply returns the original path
func RelPath(dir string) string {
	path, err := filepath.Rel(FullPath(filepath.Dir(os.Args[0])), dir)
	if err != nil {
		return dir
	}
	return path
}

// Exists reports whether a path can be statted successfully.
func Exists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

// DirEmpty checks if the given directory is empty.
func DirEmpty(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}

	_, err = f.Readdirnames(1)
	if errClose := f.Close(); errClose != nil {
		return false
	}

	return err == io.EOF
}
