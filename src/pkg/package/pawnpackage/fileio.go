package pawnpackage

import (
	stderrors "errors"
	"fmt"
	"os"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func readDefinitionFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read configuration from '%s'", path)
	}

	return data, nil
}

func writeDefinitionFile(path, label string, contents []byte) error {
	mode := os.FileMode(0o700)
	info, err := os.Lstat(path)
	if err != nil && !stderrors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to inspect %s", label)
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", label)
		}
		mode = info.Mode().Perm()
	}
	if err := fs.WriteFileAtomic(path, contents, fs.PermDirPrivate, mode); err != nil {
		return errors.Wrapf(err, "failed to write %s", label)
	}

	return nil
}
