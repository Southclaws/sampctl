package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func readCheckManagedFile(root *os.Root, path string, original os.FileInfo) (data []byte, err error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	current, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if !os.SameFile(original, current) {
		return nil, fmt.Errorf("managed cache file changed during repair")
	}

	return io.ReadAll(file)
}

func checkManagedRegularPath(cacheDir, path string) (os.FileInfo, error) {
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return nil, err
	}

	target, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(root, target)
	if err != nil {
		return nil, err
	}

	if !filepath.IsLocal(rel) || rel == "." {
		return nil, fmt.Errorf("path %q is outside the managed cache", path)
	}

	info, err := checkManagedDirectoryAncestors(root)
	if err != nil {
		return nil, err
	}

	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err = os.Lstat(current)
		if err != nil {
			return nil, err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("managed cache path %q contains a symlink", current)
		}
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("managed cache file %q is not regular", path)
	}

	return info, nil
}

func checkManagedDirectoryAncestors(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("managed cache root is not a regular directory")
	}

	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		parentInfo, err := os.Lstat(parent)
		if err != nil {
			return nil, err
		}

		if parentInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("managed cache ancestor %q is a symlink", parent)
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}

	return info, nil
}

func openCheckManagedFileRoot(cacheDir, path string) (*os.Root, string, os.FileInfo, error) {
	info, err := checkManagedRegularPath(cacheDir, path)
	if err != nil {
		return nil, "", nil, err
	}

	relative, err := filepath.Rel(cacheDir, path)
	if err != nil {
		return nil, "", nil, err
	}

	root, err := os.OpenRoot(cacheDir)
	if err != nil {
		return nil, "", nil, err
	}

	current, err := root.Lstat(relative)
	if err != nil || !os.SameFile(info, current) {
		if err == nil {
			err = fmt.Errorf("managed cache file changed during repair")
		}
		return nil, "", nil, errors.Join(err, root.Close())
	}

	return root, relative, info, nil
}
