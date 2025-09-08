package util

import "github.com/kirsle/configdir"

const (
	FOLDER_NAME = "sampctl"
)

// GetConfigDir returns the full path to the user's cache directory, creating it if it doesn't exist
func GetConfigDir() (cacheDir string) {
	cacheDir = configdir.LocalConfig(FOLDER_NAME)
	return
}
