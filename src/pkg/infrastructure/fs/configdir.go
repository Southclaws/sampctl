package fs

import (
	"fmt"

	"github.com/kirsle/configdir"
)

const configFolderName = "sampctl"

// ConfigDir returns the user's sampctl config directory and ensures it exists.
func ConfigDir() (string, error) {
	dir := configdir.LocalConfig(configFolderName)
	if err := EnsureDir(dir, PermDirPrivate); err != nil {
		return "", fmt.Errorf("failed to create config dir %q: %w", dir, err)
	}
	return dir, nil
}

// MustConfigDir wraps ConfigDir and panics on error.
func MustConfigDir() string {
	dir, err := ConfigDir()
	if err != nil {
		panic(err)
	}
	return dir
}
