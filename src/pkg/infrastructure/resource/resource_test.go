package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/github"
	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestLocalResource(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	resource := NewLocalResource(testFile, ResourceTypeArbitraryFile)

	assert.Equal(t, testFile, resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())

	cached, path := resource.Cached("any-version")
	assert.True(t, cached)
	assert.Equal(t, testFile, path)

	targetFile := filepath.Join(tmpDir, "target.txt")
	err = resource.Ensure(context.Background(), resource.Version(), targetFile)
	require.NoError(t, err)

	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestHTTPResource(t *testing.T) {
	resource := NewHTTPResource("https://example.com/test.txt", "test.txt", "1.0", ResourceTypeArbitraryFile)

	assert.Equal(t, "https://example.com/test.txt", resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())
	assert.Equal(t, "1.0", resource.Version())
	assert.Equal(t, "https://example.com/test.txt", resource.GetURL())
	assert.Equal(t, "test.txt", resource.GetFilename())
}

func TestGitResource(t *testing.T) {
	meta := versioning.DependencyMeta{
		User: "testuser",
		Repo: "testrepo",
		Tag:  "v1.0.0",
	}

	resource := NewGitResource(meta, ResourceTypePawnLibrary)

	assert.Equal(t, "github.com/testuser/testrepo", resource.Identifier())
	assert.Equal(t, ResourceTypePawnLibrary, resource.Type())
	assert.Equal(t, "v1.0.0", resource.Version())
	assert.Equal(t, meta, resource.GetDependencyMeta())
}

func TestDefaultResourceFactory(t *testing.T) {
	factory := NewDefaultResourceFactory(github.NewClient(nil))

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	resource, err := factory.FromLocal(testFile, ResourceTypeArbitraryFile)
	require.NoError(t, err)
	assert.Equal(t, testFile, resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())

	resource, err = factory.FromURL("https://example.com/test.txt", ResourceTypeArbitraryFile)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/test.txt", resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())

	resource, err = factory.FromDependencyString("user/repo:v1.0.0", ResourceTypePawnLibrary)
	require.NoError(t, err)
	assert.Equal(t, "github.com/user/repo", resource.Identifier())
	assert.Equal(t, ResourceTypePawnLibrary, resource.Type())
}

func TestDefaultResourceManager(t *testing.T) {
	factory := NewDefaultResourceFactory(github.NewClient(nil))
	manager := NewDefaultResourceManager(factory)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	resource := NewLocalResource(testFile, ResourceTypeArbitraryFile)
	manager.AddResource(resource)

	retrieved, err := manager.GetResource(testFile)
	require.NoError(t, err)
	assert.Equal(t, resource, retrieved)

	resources, err := manager.ListResources(ResourceTypeArbitraryFile)
	require.NoError(t, err)
	assert.Len(t, resources, 1)
	assert.Equal(t, resource, resources[0])

	resources, err = manager.ListResources(ResourceTypePawnLibrary)
	require.NoError(t, err)
	assert.Len(t, resources, 0)
}

func TestResourceTypes(t *testing.T) {
	types := []ResourceType{
		ResourceTypePawnLibrary,
		ResourceTypePawnScript,
		ResourceTypeServerBinary,
		ResourceTypePlugin,
		ResourceTypeCompiler,
		ResourceTypeArbitraryFile,
		ResourceTypeRustPlugin,
		ResourceTypeLuaScript,
	}

	for _, resourceType := range types {
		assert.NotEmpty(t, string(resourceType))
	}
}

func TestDefaultResourceManager_CleanCache(t *testing.T) {
	t.Cleanup(configdir.Refresh)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	cacheDir := fs.ConfigDirPath()
	if rel, err := filepath.Rel(configHome, cacheDir); err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("ConfigDirPath() = %q, want path inside temporary home %q (relative path %q, error %v)", cacheDir, configHome, rel, err)
	}

	factory := NewDefaultResourceFactory(nil)
	manager := NewDefaultResourceManager(factory)

	// Cleaning an empty temporary cache should succeed.
	err := manager.CleanCache()
	assert.NoError(t, err)
}
