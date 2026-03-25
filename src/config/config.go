package config

import (
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

// WriteConfig writes a configuration file to the given cache directory.
func WriteConfig(cacheDir string, cfg Config) error {
	return writeConfigFile(filepath.Join(cacheDir, "config.json"), cfg)
}
