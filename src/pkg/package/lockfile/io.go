package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

func Load(dir string) (*Lockfile, error) {
	path := filepath.Join(dir, Filename)
	if !util.Exists(path) {
		return nil, nil
	}
	return loadFromFile(path)
}

func loadFromFile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read lockfile")
	}

	var lockfile Lockfile
	if len(data) > 0 && data[0] == '{' {
		err = json.Unmarshal(data, &lockfile)
	} else {
		err = yaml.Unmarshal(data, &lockfile)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to parse lockfile")
	}

	if err = lockfile.Validate(); err != nil {
		return nil, errors.Wrap(err, "lockfile validation failed")
	}

	print.Verb("loaded lockfile from", path, "with", lockfile.DependencyCount(), "dependencies")
	return &lockfile, nil
}

func Save(dir string, lockfile *Lockfile, format string) error {
	if lockfile == nil {
		return errors.New("cannot save nil lockfile")
	}

	lockfile.UpdateTimestamp()

	var (
		data []byte
		err  error
	)

	switch format {
	case "yaml":
		data, err = yaml.Marshal(lockfile)
	default:
		data, err = json.MarshalIndent(lockfile, "", "\t")
	}

	if err != nil {
		return errors.Wrap(err, "failed to marshal lockfile")
	}

	path := filepath.Join(dir, Filename)
	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return errors.Wrap(err, "failed to write lockfile")
	}

	print.Verb("saved lockfile to", path, "with", lockfile.DependencyCount(), "dependencies")
	return nil
}

func Exists(dir string) bool {
	path := filepath.Join(dir, Filename)
	return util.Exists(path)
}

func GetPath(dir string) string {
	path := filepath.Join(dir, Filename)
	if util.Exists(path) {
		return path
	}
	return ""
}

func Delete(dir string) error {
	path := filepath.Join(dir, Filename)
	if !util.Exists(path) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return errors.Wrap(err, "failed to remove lockfile")
	}
	return nil
}

func LoadOrCreate(dir, sampctlVersion string) (*Lockfile, error) {
	existing, err := Load(dir)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return New(sampctlVersion), nil
}