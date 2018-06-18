package types

import (
	"encoding/json"
	"io/ioutil"
	"os/user"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/Southclaws/sampctl/util"
)

// Config represents a local configuration for sampctl
type Config struct {
	UserID      string `json:"user_id"`
	Metrics     bool   `json:"metrics"`
	DefaultUser string `json:"default_user"`
	GitHubToken string `json:"github_token,omitempty"`
	GitUsername string `json:"git_username,omitempty"`
	GitPassword string `json:"git_password,omitempty"`
	NewUser     bool   `json:"-"`
}

// LoadOrCreateConfig reads a config file from the given cache directory
func LoadOrCreateConfig(cacheDir string) (cfg *Config, err error) {
	configFile := filepath.Join(cacheDir, "config.json")
	cfg = new(Config)

	var contents []byte
	if util.Exists(configFile) {
		contents, err = ioutil.ReadFile(configFile)
		if err != nil {
			return
		}

		err = json.Unmarshal(contents, &cfg)
		if err != nil {
			return
		}

		if cfg.UserID == "" {
			cfg.UserID = uuid.New().String()
			cfg.Metrics = true
			cfg.NewUser = true
		}
	} else {
		var (
			u        *user.User
			username string
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
		err = ioutil.WriteFile(configFile, contents, 0666)
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
