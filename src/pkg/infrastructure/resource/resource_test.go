package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestLocalResource(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Create local resource
	resource := NewLocalResource(testFile, ResourceTypeArbitraryFile)
	
	assert.Equal(t, testFile, resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())
	
	// Test cached method
	cached, path := resource.Cached("any-version")
	assert.True(t, cached)
	assert.Equal(t, testFile, path)
	
	// Test ensure to a target path
	targetFile := filepath.Join(tmpDir, "target.txt")
	err = resource.Ensure(context.Background(), resource.Version(), targetFile)
	require.NoError(t, err)
	
	// Verify target file exists and has correct content
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestHTTPResource(t *testing.T) {
	// This test would require a real HTTP server or mocking
	// For now, just test creation
	resource := NewHTTPResource("https://example.com/test.txt", "test.txt", "1.0", ResourceTypeArbitraryFile)
	
	assert.Equal(t, "https://example.com/test.txt", resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())
	assert.Equal(t, "1.0", resource.Version())
	assert.Equal(t, "https://example.com/test.txt", resource.GetURL())
	assert.Equal(t, "test.txt", resource.GetFilename())
}

func TestGitResource(t *testing.T) {
	// Test creation from dependency meta
	meta := versioning.DependencyMeta{
		User: "testuser",
		Repo: "testrepo",
		Tag:  "v1.0.0",
	}
	
	resource := NewGitResource(meta, ResourceTypePawnLibrary)
	
	assert.Equal(t, "testuser/testrepo:v1.0.0", resource.Identifier())
	assert.Equal(t, ResourceTypePawnLibrary, resource.Type())
	assert.Equal(t, "v1.0.0", resource.Version())
	assert.Equal(t, meta, resource.GetDependencyMeta())
}

func TestDefaultResourceFactory(t *testing.T) {
	factory := NewDefaultResourceFactory(github.NewClient(nil))
	
	// Test FromLocal
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)
	
	resource, err := factory.FromLocal(testFile, ResourceTypeArbitraryFile)
	require.NoError(t, err)
	assert.Equal(t, testFile, resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())
	
	// Test FromURL
	resource, err = factory.FromURL("https://example.com/test.txt", ResourceTypeArbitraryFile)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/test.txt", resource.Identifier())
	assert.Equal(t, ResourceTypeArbitraryFile, resource.Type())
	
	// Test FromDependencyString
	resource, err = factory.FromDependencyString("user/repo:v1.0.0", ResourceTypePawnLibrary)
	require.NoError(t, err)
	assert.Equal(t, "github.com/user/repo:v1.0.0", resource.Identifier())
	assert.Equal(t, ResourceTypePawnLibrary, resource.Type())
}

func TestDefaultResourceManager(t *testing.T) {
	factory := NewDefaultResourceFactory(github.NewClient(nil))
	manager := NewDefaultResourceManager(factory)
	
	// Create and add a resource
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)
	
	resource := NewLocalResource(testFile, ResourceTypeArbitraryFile)
	manager.AddResource(resource)
	
	// Test GetResource
	retrieved, err := manager.GetResource(testFile)
	require.NoError(t, err)
	assert.Equal(t, resource, retrieved)
	
	// Test ListResources
	resources, err := manager.ListResources(ResourceTypeArbitraryFile)
	require.NoError(t, err)
	assert.Len(t, resources, 1)
	assert.Equal(t, resource, resources[0])
	
	// Test ListResources with different type
	resources, err = manager.ListResources(ResourceTypePawnLibrary)
	require.NoError(t, err)
	assert.Len(t, resources, 0)
}

func TestResourceTypes(t *testing.T) {
	// Test that all resource types are properly defined
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
	// Create mock factory
	factory := NewDefaultResourceFactory(nil)
	manager := NewDefaultResourceManager(factory)
	
	// Test that CleanCache doesn't crash - the actual cleaning logic
	// will depend on the real cache directory structure
	err := manager.CleanCache()
	assert.NoError(t, err)
}
