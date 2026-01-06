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

// Load attempts to load a lockfile from the given directory.
// Format is auto-detected from file content. Returns nil if no lockfile exists.
func Load(dir string) (*Lockfile, error) {
	path := filepath.Join(dir, Filename)

	if !util.Exists(path) {
		return nil, nil
	}

	return loadFromFile(path)
}

// loadFromFile loads a lockfile from a specific file path.
// Format is auto-detected from content (JSON starts with '{', otherwise YAML).
func loadFromFile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read lockfile")
	}

	var lockfile Lockfile

	// Auto-detect format: JSON starts with '{', otherwise try YAML
	if len(data) > 0 && data[0] == '{' {
		err = json.Unmarshal(data, &lockfile)
	} else {
		err = yaml.Unmarshal(data, &lockfile)
	}

	if err != nil {
		return nil, errors.Wrap(err, "failed to parse lockfile")
	}

	// Validate the loaded lockfile
	if err = lockfile.Validate(); err != nil {
		return nil, errors.Wrap(err, "lockfile validation failed")
	}

	print.Verb("loaded lockfile from", path, "with", lockfile.DependencyCount(), "dependencies")
	return &lockfile, nil
}

// Save writes the lockfile to the given directory.
// The format parameter determines encoding (json or yaml), but filename is always pawn.lock.
func Save(dir string, lockfile *Lockfile, format string) error {
	if lockfile == nil {
		return errors.New("cannot save nil lockfile")
	}

	// Update timestamp before saving
	lockfile.UpdateTimestamp()

	var (
		data []byte
		err  error
	)

	switch format {
	case "yaml":
		data, err = yaml.Marshal(lockfile)
	default:
		// Default to JSON
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

// Exists checks if a lockfile exists in the given directory
func Exists(dir string) bool {
	path := filepath.Join(dir, Filename)
	return util.Exists(path)
}

// GetPath returns the path to the lockfile if it exists
func GetPath(dir string) string {
	path := filepath.Join(dir, Filename)
	if util.Exists(path) {
		return path
	}
	return ""
}

// Delete removes the lockfile from the given directory
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

// LoadOrCreate loads an existing lockfile or creates a new one
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