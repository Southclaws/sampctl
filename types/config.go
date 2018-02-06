package types

import (
	"encoding/json"
	"io/ioutil"
	"os/user"
	"path/filepath"

	"github.com/Southclaws/sampctl/util"
)

// Config represents a local configuration for sampctl
type Config struct {
	DefaultUser string `json:"default_user"`
	GitHubToken string `json:"github_token,omitempty"`
	GitUsername string `json:"git_username,omitempty"`
	GitPassword string `json:"git_password,omitempty"`
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
	} else {
		var (
			u        *user.User
			username string
		)
		u, err = user.Current()
		if err != nil {
			err = nil // ignore error
			username = ""
		} else {
			username = u.Username
		}
		cfg.DefaultUser = username
		contents, err = json.Marshal(cfg)
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
	contents, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(configFile, contents, 0666)
	if err != nil {
		return
	}
	return
}
