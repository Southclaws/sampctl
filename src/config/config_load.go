package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kr/pretty"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func loadDotEnv() {
	err := godotenv.Load(".env")
	if err != nil && !strings.HasPrefix(err.Error(), "open .env") {
		print.Warn("Failed to load .env:", err)
	}
}

func findConfigFile(cacheDir string) (string, bool) {
	for _, file := range configFileCandidates(cacheDir) {
		if fs.Exists(file) {
			return file, true
		}
	}

	return "", false
}

func configFileCandidates(cacheDir string) []string {
	return []string{
		filepath.Join(cacheDir, "config.json"),
		filepath.Join(cacheDir, "config.yaml"),
	}
}

func loadConfigFile(configFile string) (*Config, error) {
	contents, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	cfg := new(Config)
	if err := unmarshalConfigFile(configFile, contents, cfg); err != nil {
		return nil, err
	}
	if err := applyEnvironmentOverrides(cfg); err != nil {
		return nil, err
	}

	normalizeConfig(cfg)
	print.Verb("Using configuration:", pretty.Sprint(cfg))

	return cfg, nil
}

func createDefaultConfig(cacheDir string) (*Config, error) {
	print.Verb("No configuration file found, using default configuration")

	cfg := defaultConfig(lookupCurrentUsername())
	if err := writeConfigFile(filepath.Join(cacheDir, "config.json"), cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func unmarshalConfigFile(configFile string, contents []byte, cfg *Config) error {
	switch strings.ToLower(filepath.Ext(configFile)) {
	case ".json":
		return json.Unmarshal(contents, cfg)
	case ".yaml", ".yml":
		return yaml.Unmarshal(contents, cfg)
	default:
		return fmt.Errorf("unsupported config format %q", filepath.Ext(configFile))
	}
}

func applyEnvironmentOverrides(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if value, ok := os.LookupEnv("SAMPCTL_DEFAULT_USER"); ok {
		cfg.DefaultUser = value
	}
	if value, ok := os.LookupEnv("SAMPCTL_GITHUB_TOKEN"); ok {
		cfg.GitHubToken = value
	}
	if value, ok := os.LookupEnv("SAMPCTL_GIT_USERNAME"); ok {
		cfg.GitUsername = value
	}
	if value, ok := os.LookupEnv("SAMPCTL_GIT_PASSWORD"); ok {
		cfg.GitPassword = value
	}
	if value, ok := os.LookupEnv("SAMPCTL_HIDE_VERSION_UPDATE_MESSAGE"); ok {
		parsed, err := parseConfigBool(value)
		if err != nil {
			return fmt.Errorf("failed to parse SAMPCTL_HIDE_VERSION_UPDATE_MESSAGE: %w", err)
		}
		cfg.HideVersionUpdateMessage = &parsed
	}
	if value, ok := os.LookupEnv("CI"); ok {
		cfg.CI = value
	}

	return nil
}

func parseConfigBool(value string) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}

	return strconv.ParseBool(value)
}
