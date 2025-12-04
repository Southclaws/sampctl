package pawnpackage

import (
	"os"

	"github.com/pkg/errors"
)

func readDefinitionFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read configuration from '%s'", path)
	}

	return data, nil
}

func writeDefinitionFile(path, label string, contents []byte) error {
	if err := os.WriteFile(path, contents, 0o700); err != nil {
		return errors.Wrapf(err, "failed to write %s", label)
	}

	return nil
}
