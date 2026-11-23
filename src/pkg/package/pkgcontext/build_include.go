package pkgcontext

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func publishBuildInclude(path string, content []byte) (err error) {
	dir, name := filepath.Dir(path), filepath.Base(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()

	previous, err := buildIncludeInfo(root, name, path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if previous != nil {
		current, err := root.ReadFile(name)
		if err != nil {
			return err
		}
		if bytes.Equal(current, content) {
			return nil
		}
		mode = previous.Mode().Perm()
	}

	stagedName := "." + name + "." + rand.Text()
	staged, err := root.OpenFile(stagedName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if stagedName != "" {
			if removeErr := root.Remove(stagedName); !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
	}()

	if err := writeBuildInclude(staged, content, mode); err != nil {
		return err
	}
	if _, err := buildIncludeInfo(root, name, path); err != nil {
		return err
	}
	if err := root.Rename(stagedName, name); err != nil {
		return err
	}
	stagedName = ""
	return fs.SyncDir(dir)
}

func buildIncludeInfo(root *os.Root, name, path string) (os.FileInfo, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, &os.PathError{Op: "publish", Path: path, Err: errors.New("build include is not a regular file")}
	}
	return info, nil
}

func writeBuildInclude(staged *os.File, content []byte, mode os.FileMode) (err error) {
	defer func() { err = errors.Join(err, staged.Close()) }()

	if _, err := staged.Write(content); err != nil {
		return err
	}
	if err := staged.Chmod(mode); err != nil {
		return err
	}
	return staged.Sync()
}
