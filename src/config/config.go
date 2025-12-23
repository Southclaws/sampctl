package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kr/pretty"
	"github.com/sampctl/configor"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
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

// LoadOrCreateConfig reads a config file from the given cache directory
func LoadOrCreateConfig(cacheDir string, verbose bool) (cfg *Config, err error) {
	cfg = new(Config)

	err = godotenv.Load(".env")
	// on unix: "open .env: no such file or directory"
	// on windows: "open .env: The system cannot find the file specified"
	if err != nil && !strings.HasPrefix(err.Error(), "open .env") {
		print.Warn("Failed to load .env:", err)
	}

	configFiles := []string{
		filepath.Join(cacheDir, "config.json"),
		filepath.Join(cacheDir, "config.yaml"),
	}
	configFile := ""
	for _, file := range configFiles {
		if fs.Exists(file) {
			configFile = file
			break
		}
	}

	if configFile != "" {
		cnfgr := configor.New(&configor.Config{
			EnvironmentPrefix:    "SAMPCTL",
			ErrorOnUnmatchedKeys: false,
		})

		err = cnfgr.Load(cfg, configFile)
		if err != nil {
			return nil, err
		}

		if cfg.HideVersionUpdateMessage == nil {
			value := false
			cfg.HideVersionUpdateMessage = &value
		}
		print.Verb("Using configuration:", pretty.Sprint(cfg))
	} else {
		print.Verb("No configuration file found, using default configuration")
		var (
			u        *user.User
			username string
			contents []byte
		)
		u, err = user.Current()
		if err != nil {
			username = ""
		} else {
			username = u.Username
		}

		value := false
		cfg.HideVersionUpdateMessage = &value

		cfg.DefaultUser = username
		contents, err = json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			return
		}
		err = os.WriteFile(configFiles[0], contents, fs.PermFileShared)
		if err != nil {
			return
		}
	}

	return cfg, nil
}

// WriteConfig writes a configuration file to the given cache directory
func WriteConfig(cacheDir string, cfg Config) (err error) {
	configFile := filepath.Join(cacheDir, "config.json")
	contents, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return
	}
	err = os.WriteFile(configFile, contents, fs.PermFileShared)
	if err != nil {
		return
	}
	return
}
