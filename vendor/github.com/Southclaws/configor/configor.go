package configor

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

// Configor represents a configuration loader with its own behaviour config
type Configor struct {
	*Config
}

// Config is the configuration loader's own configuration
type Config struct {
	Environment       string
	EnvironmentPrefix string
	Verbose           bool

	// In case of json files, this field will be used only when compiled with
	// go 1.10 or later.
	// This field will be ignored when compiled with go versions lower than 1.10.
	ErrorOnUnmatchedKeys bool
}

// New initializes a configuration loader
func New(config *Config) *Configor {
	if config == nil {
		config = &Config{}
	}
	return &Configor{Config: config}
}

var (
	// ErrNoConfigFiles is returned by Load when no files from the given list
	// could be found.
	ErrNoConfigFiles = errors.New("could not find any configuration files")
)

// Load will decode the given files into `config` using default settings.
func Load(config interface{}, files ...string) (err error) {
	return New(nil).Load(config, files...)
}

// Load will decode the given files into `config`
func (configor *Configor) Load(config interface{}, files ...string) (err error) {
	configFiles := configor.getConfigurationFiles(files...)
	if len(configFiles) == 0 {
		return ErrNoConfigFiles
	}

	for _, file := range configFiles {
		if configor.Config.Verbose {
			fmt.Println("loading configuration from", file)
		}
		if err := UnmarshalFile(config, file, configor.ErrorOnUnmatchedKeys); err != nil {
			return err
		}
	}

	prefix := configor.EnvironmentPrefix
	if prefix == "" {
		return configor.processTags(config)
	}
	return configor.processTags(config, prefix)
}

// UnmatchedTomlKeysError errors are returned by the Load function when
// ErrorOnUnmatchedKeys is set to true and there are unmatched keys in the input
// toml config file. The string returned by Error() contains the names of the
// missing keys.
type UnmatchedTomlKeysError struct {
	Keys []toml.Key
}

func (e *UnmatchedTomlKeysError) Error() string {
	return fmt.Sprintf("There are keys in the config file that do not match any field in the given struct: %v", e.Keys)
}

func getFilenameWithEnvironmentPrefix(file, env string) (string, error) {
	var (
		envFile string
		extname = path.Ext(file)
	)

	if extname == "" {
		envFile = fmt.Sprintf("%v.%v", file, env)
	} else {
		envFile = fmt.Sprintf("%v.%v%v", strings.TrimSuffix(file, extname), env, extname)
	}

	if fileInfo, err := os.Stat(envFile); err == nil && fileInfo.Mode().IsRegular() {
		return envFile, nil
	}
	return "", errors.Errorf("failed to find file %v", file)
}

func (configor *Configor) getConfigurationFiles(files ...string) []string {
	var results []string

	for i := len(files) - 1; i >= 0; i-- {
		foundFile := false
		file := files[i]

		// check configuration
		if fileInfo, err := os.Stat(file); err == nil && fileInfo.Mode().IsRegular() {
			foundFile = true
			results = append(results, file)
		}

		// check configuration with env
		if fileWithPrefix, err := getFilenameWithEnvironmentPrefix(file, configor.Environment); err == nil {
			foundFile = true
			results = append(results, fileWithPrefix)
		}

		// check example configuration
		if !foundFile {
			if fileWithPrefix, err := getFilenameWithEnvironmentPrefix(file, "example"); err == nil {
				results = append(results, fileWithPrefix)
			}
		}
	}
	return results
}
