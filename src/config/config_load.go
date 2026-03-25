package config

import (
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kr/pretty"
	"github.com/sampctl/configor"

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
	cfg := new(Config)
	cnfgr := configor.New(&configor.Config{
		EnvironmentPrefix:    "SAMPCTL",
		ErrorOnUnmatchedKeys: false,
	})

	if err := cnfgr.Load(cfg, configFile); err != nil {
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
