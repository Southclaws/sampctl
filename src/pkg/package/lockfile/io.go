package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func Load(dir string) (*Lockfile, error) {
	path := filepath.Join(dir, Filename)
	if !fs.Exists(path) {
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
	err = json.Unmarshal(data, &lockfile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse lockfile")
	}

	if err = lockfile.Validate(); err != nil {
		return nil, errors.Wrap(err, "lockfile validation failed")
	}

	print.Verb("loaded lockfile from", path, "with", lockfile.DependencyCount(), "dependencies")
	return &lockfile, nil
}

func Save(dir string, lockfile *Lockfile) error {
	if lockfile == nil {
		return errors.New("cannot save nil lockfile")
	}

	lockfile.UpdateTimestamp()

	data, err := json.MarshalIndent(lockfile, "", "\t")
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
	return fs.Exists(path)
}

func GetPath(dir string) string {
	path := filepath.Join(dir, Filename)
	if fs.Exists(path) {
		return path
	}
	return ""
}

func Delete(dir string) error {
	path := filepath.Join(dir, Filename)
	if !fs.Exists(path) {
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
