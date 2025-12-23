package versioning

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	originalLoader := remoteOverridesLoader
	remoteOverridesLoader = func() map[string]string {
		return map[string]string{}
	}
	ResetDependencyOverrides()
	code := m.Run()
	remoteOverridesLoader = originalLoader
	ResetDependencyOverrides()
	os.Exit(code)
}

func TestApplyDependencyOverrides(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exact match with github.com prefix",
			input:    "github.com/Zeex/samp-plugin-crashdetect",
			expected: "github.com/AmyrAhmady/samp-plugin-crashdetect",
		},
		{
			name:     "exact match without prefix",
			input:    "Zeex/samp-plugin-crashdetect",
			expected: "AmyrAhmady/samp-plugin-crashdetect",
		},
		{
			name:     "https url match",
			input:    "https://github.com/Zeex/samp-plugin-crashdetect",
			expected: "github.com/AmyrAhmady/samp-plugin-crashdetect",
		},
		{
			name:     "http url match",
			input:    "http://github.com/Zeex/samp-plugin-crashdetect",
			expected: "github.com/AmyrAhmady/samp-plugin-crashdetect",
		},
		{
			name:     "with tag should preserve tag",
			input:    "Zeex/samp-plugin-crashdetect:v1.0.0",
			expected: "AmyrAhmady/samp-plugin-crashdetect:v1.0.0",
		},
		{
			name:     "with branch should preserve branch",
			input:    "Zeex/samp-plugin-crashdetect@main",
			expected: "AmyrAhmady/samp-plugin-crashdetect@main",
		},
		{
			name:     "with commit should preserve commit",
			input:    "Zeex/samp-plugin-crashdetect#1234567890123456789012345678901234567890",
			expected: "AmyrAhmady/samp-plugin-crashdetect#1234567890123456789012345678901234567890",
		},
		{
			name:     "no override for non-matching dependency",
			input:    "SomeUser/some-other-repo",
			expected: "SomeUser/some-other-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyDependencyOverrides(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyDependencyOverrides_ReplacementWithVersionDoesNotAppendOriginal(t *testing.T) {
	ResetDependencyOverrides()

	loadedOverrides = map[string]string{
		"Zeex/samp-plugin-crashdetect": "AmyrAhmady/samp-plugin-crashdetect:v4.22",
	}

	result := ApplyDependencyOverrides("Zeex/samp-plugin-crashdetect:v4.20")
	assert.Equal(t, "AmyrAhmady/samp-plugin-crashdetect:v4.22", result)

	ResetDependencyOverrides()
}

func TestDependencyOverrideIntegration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected DependencyMeta
	}{
		{
			name:  "override applied during explode",
			input: "Zeex/samp-plugin-crashdetect",
			expected: DependencyMeta{
				Site: "github.com",
				User: "AmyrAhmady",
				Repo: "samp-plugin-crashdetect",
			},
		},
		{
			name:  "override with tag",
			input: "Zeex/samp-plugin-crashdetect:v1.0.0",
			expected: DependencyMeta{
				Site: "github.com",
				User: "AmyrAhmady",
				Repo: "samp-plugin-crashdetect",
				Tag:  "v1.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep, err := DependencyString(tt.input).Explode()
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Site, dep.Site)
			assert.Equal(t, tt.expected.User, dep.User)
			assert.Equal(t, tt.expected.Repo, dep.Repo)
			assert.Equal(t, tt.expected.Tag, dep.Tag)
		})
	}
}

func TestDependencyOverrideConfig(t *testing.T) {
	// Create a temporary directory for test config
	tmpDir, err := os.MkdirTemp("", "sampctl-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "dependency-overrides.json")

	// Test saving and loading config
	testOverrides := map[string]string{
		"old-user/old-repo":  "new-user/new-repo",
		"github.com/foo/bar": "github.com/baz/qux",
		"deprecated/package": "maintained/package",
	}

	// Save config
	err = SaveDependencyOverrides(testOverrides, configPath)
	require.NoError(t, err)

	// Load config
	loadedOverrides := LoadDependencyOverrides(configPath)

	// Check that loaded overrides contain both built-in and user-defined overrides
	for original, replacement := range DependencyOverrides {
		assert.Equal(t, replacement, loadedOverrides[original], "built-in override should be preserved")
	}

	for original, replacement := range testOverrides {
		assert.Equal(t, replacement, loadedOverrides[original], "user-defined override should be loaded")
	}
}

func TestLoadDependencyOverridesWithNonexistentFile(t *testing.T) {
	// Test loading from a file that doesn't exist
	loadedOverrides := LoadDependencyOverrides("/nonexistent/path/config.json")

	// Should return built-in overrides
	for original, replacement := range DependencyOverrides {
		assert.Equal(t, replacement, loadedOverrides[original])
	}
}

func TestApplyDependencyOverridesWithConfig(t *testing.T) {
	// Reset the global loaded overrides
	ResetDependencyOverrides()

	// Create a temporary config with custom overrides
	tmpDir, err := os.MkdirTemp("", "sampctl-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "dependency-overrides.json")
	customOverrides := map[string]string{
		"custom/dependency": "replacement/dependency",
	}

	err = SaveDependencyOverrides(customOverrides, configPath)
	require.NoError(t, err)

	// Test that the function works with built-in overrides
	result := ApplyDependencyOverrides("Zeex/samp-plugin-crashdetect")
	assert.Equal(t, "AmyrAhmady/samp-plugin-crashdetect", result)

	// Reset for other tests
	ResetDependencyOverrides()
}
