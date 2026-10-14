package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents a local configuration for sampctl
// nolint:lll
type Config struct {
	DefaultUser              string `json:"default_user"           env:"SAMPCTL_DEFAULT_USER"`                               // the default username for `package init`
	GitHubToken              string `json:"github_token,omitempty" env:"SAMPCTL_GITHUB_TOKEN"`                               // GitHub API token for extended API rate limit
	GitUsername              string `json:"git_username,omitempty" env:"SAMPCTL_GIT_USERNAME"`                               // Git username for private repositories
	GitPassword              string `json:"git_password,omitempty" env:"SAMPCTL_GIT_PASSWORD"`                               // Git password for private repositories
	HideVersionUpdateMessage *bool  `json:"hide_version_update_message,omitempty" env:"SAMPCTL_HIDE_VERSION_UPDATE_MESSAGE"` // Hides the version update message reminder
	CI                       string `json:"-" yaml:"-"             env:"CI"`                                                 // So sampctl can detect if it's running inside GitLab CI/CD or TravisCI
}

// LoadOrCreateConfig reads a config file from the given cache directory.
func LoadOrCreateConfig(cacheDir string) (*Config, error) {
	loadDotEnv()

	configFile, ok := findConfigFile(cacheDir)
	if ok {
		return loadConfigFile(configFile)
	}

	return createDefaultConfig(cacheDir)
}

// LoadConfig reads existing configuration or uses defaults without writing files.
func LoadConfig(cacheDir string) (*Config, error) {
	for _, path := range configFileCandidates(cacheDir) {
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect configuration %q: %w", path, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			info, err = os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("inspect configuration %q: %w", path, err)
			}
		}

		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("configuration %q is not a regular file", path)
		}

		return readConfigFile(path)
	}

	cfg := defaultConfig(lookupCurrentUsername())
	if err := applyEnvironmentOverrides(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// WriteConfig writes a configuration file to the given cache directory.
func WriteConfig(cacheDir string, cfg Config) error {
	return writeConfigFile(filepath.Join(cacheDir, "config.json"), cfg)
}
