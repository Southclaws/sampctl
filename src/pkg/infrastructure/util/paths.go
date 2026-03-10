package util

import "github.com/kirsle/configdir"

const (
	FolderName = "sampctl"
)

// GetConfigDir returns the full path to the user's cache directory, creating it if it doesn't exist
func GetConfigDir() (cacheDir string) {
	cacheDir = configdir.LocalConfig(FolderName)
	return
}
