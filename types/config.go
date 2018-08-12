package types

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/configor"
	"github.com/joho/godotenv"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
)

// Config represents a local configuration for sampctl
type Config struct {
	UserID      string `json:"user_id"                env:"__do_not_set__"`       // Anonymous user ID for metrics
	Metrics     bool   `json:"metrics"                env:"SAMPCTL_METRICS"`      // Whether or not to report telemetry metrics
	DefaultUser string `json:"default_user"           env:"SAMPCTL_DEFAULT_USER"` // the default username for `package init`
	GitHubToken string `json:"github_token,omitempty" env:"SAMPCTL_GITHUB_TOKEN"` // GitHub API token for extended API rate limit
	GitUsername string `json:"git_username,omitempty" env:"SAMPCTL_GIT_USERNAME"` // Git username for private repositories
	GitPassword string `json:"git_password,omitempty" env:"SAMPCTL_GIT_PASSWORD"` // Git password for private repositories
	CI          string `json:"-" yaml:"-"             env:"CI"`                   // So sampctl can detect if it's running inside GitLab CI/CD or TravisCI
	NewUser     bool   `json:"-" yaml:"-"             env:"__do_not_set__"`       // (only used internally) whether or not it's the first-run
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
		filepath.Join(cacheDir, "config.toml"),
	}
	configFile := ""
	for _, file := range configFiles {
		if util.Exists(file) {
			configFile = file
			break
		}
	}

	if configFile != "" {
		cnfgr := configor.New(&configor.Config{
			ENVPrefix:            "SAMPCTL",
			Debug:                os.Getenv("DEBUG") != "",
			Verbose:              verbose,
			ErrorOnUnmatchedKeys: true,
		})

		err = cnfgr.Load(cfg, configFile)
		if err != nil {
			return nil, err
		}

		if cfg.UserID == "" {
			cfg.UserID = uuid.New().String()
			cfg.Metrics = true
			cfg.NewUser = true
		}
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

		cfg.UserID = uuid.New().String()
		cfg.Metrics = true
		cfg.NewUser = true

		cfg.DefaultUser = username
		contents, err = json.MarshalIndent(cfg, "", "    ")
		if err != nil {
			return
		}
		err = ioutil.WriteFile(configFiles[0], contents, 0666)
		if err != nil {
			return
		}
	}

	return
}

// WriteConfig writes a configuration file to the given cache directory
func WriteConfig(cacheDir string, cfg Config) (err error) {
	configFile := filepath.Join(cacheDir, "config.json")
	contents, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return
	}
	err = ioutil.WriteFile(configFile, contents, 0666)
	if err != nil {
		return
	}
	return
}
