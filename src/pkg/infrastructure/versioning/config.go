package versioning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/cache"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

// DependencyOverrideConfig represents the structure of a dependency override configuration file
type DependencyOverrideConfig struct {
	Overrides map[string]string `json:"overrides"`
}

const (
	// RemoteOverridesURL is the URL to fetch dependency overrides from
	RemoteOverridesURL = "https://raw.githubusercontent.com/sampctl/plugins/refs/heads/master/dependency-overrides.json"
	// CacheValidityDuration is how long the cached overrides are valid
	CacheValidityDuration = 24 * time.Hour
)

var remoteOverridesLoader = defaultRemoteOverridesLoader

// DefaultDependencyOverridesPath returns the default path for the dependency overrides configuration file
func DefaultDependencyOverridesPath() string {
	return filepath.Join(util.GetConfigDir(), "dependency-overrides.json")
}

// DefaultDependencyOverridesCachePath returns the default path for the cached remote dependency overrides
func DefaultDependencyOverridesCachePath() string {
	return filepath.Join(util.GetConfigDir(), "remote-dependency-overrides.json")
}

// isCacheValid checks if the cached file exists and is within the validity period
func isCacheValid(cachePath string) bool {
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < CacheValidityDuration
}

// downloadRemoteOverrides downloads dependency overrides from the remote URL
func downloadRemoteOverrides(url, cachePath string) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download overrides: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download overrides: HTTP %d", resp.StatusCode)
	}
	if err := cache.WriteFromReaderAtomic(cachePath, resp.Body, 0o755, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}
	return nil
}

func loadRemoteOverrides() map[string]string {
	return remoteOverridesLoader()
}

// defaultRemoteOverridesLoader downloads dependency overrides from the remote URL with caching.
func defaultRemoteOverridesLoader() map[string]string {
	cachePath := DefaultDependencyOverridesCachePath()

	if !isCacheValid(cachePath) {
		if err := downloadRemoteOverrides(RemoteOverridesURL, cachePath); err != nil {
			return make(map[string]string)
		}
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return make(map[string]string)
	}

	var config DependencyOverrideConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return make(map[string]string)
	}

	return config.Overrides
}

// LoadDependencyOverrides loads dependency overrides from multiple sources:
// 1. Built-in overrides
// 2. Remote overrides (with caching)
// 3. Local configuration file
// Later sources override earlier ones
func LoadDependencyOverrides(configPath string) map[string]string {
	// Start with built-in overrides
	overrides := make(map[string]string)
	for k, v := range DependencyOverrides {
		overrides[k] = v
	}

	// Load and merge remote overrides
	remoteOverrides := loadRemoteOverrides()
	for original, replacement := range remoteOverrides {
		overrides[original] = replacement
	}

	if configPath == "" {
		configPath = DefaultDependencyOverridesPath()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return overrides
	}

	var config DependencyOverrideConfig
	if err := json.Unmarshal(data, &config); err != nil {
		// If parsing fails, just return current overrides
		return overrides
	}

	// Merge local overrides (they take highest precedence)
	for original, replacement := range config.Overrides {
		overrides[original] = replacement
	}

	return overrides
}

// SaveDependencyOverrides saves dependency overrides to a configuration file
func SaveDependencyOverrides(overrides map[string]string, configPath string) error {
	if configPath == "" {
		configPath = DefaultDependencyOverridesPath()
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	config := DependencyOverrideConfig{
		Overrides: overrides,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o644)
}

// ClearRemoteOverridesCache removes the cached remote overrides file
// This is useful for testing or when you want to force a fresh download
func ClearRemoteOverridesCache() error {
	cachePath := DefaultDependencyOverridesCachePath()
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil // File doesn't exist, nothing to clear
	}
	return os.Remove(cachePath)
}
