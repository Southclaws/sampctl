package resource

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// DefaultResourceFactory provides a default implementation of ResourceFactory
type DefaultResourceFactory struct {
	ghClient *github.Client
}

// NewDefaultResourceFactory creates a new DefaultResourceFactory
func NewDefaultResourceFactory(ghClient *github.Client) *DefaultResourceFactory {
	return &DefaultResourceFactory{
		ghClient: ghClient,
	}
}

// FromDependencyString creates a resource from a dependency string
func (f *DefaultResourceFactory) FromDependencyString(depString string, resourceType ResourceType) (Resource, error) {
	meta, err := versioning.DependencyString(depString).Explode()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse dependency string")
	}

	// Handle URL-like schemes
	if meta.Scheme != "" {
		if meta.Local != "" {
			// Local scheme
			return NewLocalResource(meta.Local, resourceType), nil
		}
		// Remote scheme - treat as Git repository
		return NewGitResource(meta, resourceType), nil
	}

	// Regular Git repository
	return NewGitResource(meta, resourceType), nil
}

// FromURL creates a resource from a direct URL
func (f *DefaultResourceFactory) FromURL(urlStr string, resourceType ResourceType) (Resource, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL")
	}

	// Check if it's a GitHub URL that might be a release asset
	if strings.Contains(parsedURL.Host, "github.com") && strings.Contains(parsedURL.Path, "/releases/") {
		// Try to parse as GitHub release asset
		pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
		if len(pathParts) >= 4 && pathParts[2] == "releases" {
			user := pathParts[0]
			repo := pathParts[1]

			meta := versioning.DependencyMeta{
				Site: "github.com",
				User: user,
				Repo: repo,
			}

			// Create a pattern that matches any asset (could be improved with specific pattern)
			pattern := regexp.MustCompile(".*")

			return NewGitHubReleaseResource(meta, pattern, resourceType, f.ghClient), nil
		}
	}

	// Default to HTTP resource
	return NewHTTPResource(urlStr, "", "", resourceType), nil
}

// FromLocal creates a resource from a local path
func (f *DefaultResourceFactory) FromLocal(path string, resourceType ResourceType) (Resource, error) {
	return NewLocalResource(path, resourceType), nil
}

// DefaultResourceManager provides a default implementation of ResourceManager
type DefaultResourceManager struct {
	resources map[string]Resource
	factory   ResourceFactory
}

// NewDefaultResourceManager creates a new DefaultResourceManager
func NewDefaultResourceManager(factory ResourceFactory) *DefaultResourceManager {
	return &DefaultResourceManager{
		resources: make(map[string]Resource),
		factory:   factory,
	}
}

// GetResource retrieves a resource by identifier
func (m *DefaultResourceManager) GetResource(identifier string) (Resource, error) {
	resource, exists := m.resources[identifier]
	if !exists {
		return nil, errors.Errorf("resource not found: %s", identifier)
	}
	return resource, nil
}

// AddResource adds a resource to the manager
func (m *DefaultResourceManager) AddResource(resource Resource) {
	m.resources[resource.Identifier()] = resource
}

// ListResources lists all available resources of a given type
func (m *DefaultResourceManager) ListResources(resourceType ResourceType) ([]Resource, error) {
	var result []Resource
	for _, resource := range m.resources {
		if resource.Type() == resourceType {
			result = append(result, resource)
		}
	}
	return result, nil
}

// EnsureAll ensures all resources in a dependency tree
func (m *DefaultResourceManager) EnsureAll(ctx context.Context, resources []Resource) error {
	for _, resource := range resources {
		err := resource.Ensure(ctx, resource.Version(), "")
		if err != nil {
			return errors.Wrapf(err, "failed to ensure resource: %s", resource.Identifier())
		}
	}
	return nil
}

// CleanCache removes unused cached resources
func (m *DefaultResourceManager) CleanCache() error {
	cacheDir := util.GetConfigDir()

	// Walk through all resource type directories in cache
	resourceTypes := []ResourceType{
		ResourceTypePawnLibrary,
		ResourceTypePawnScript,
		ResourceTypeServerBinary,
		ResourceTypePlugin,
		ResourceTypeCompiler,
		ResourceTypeArbitraryFile,
		ResourceTypeRustPlugin,
		ResourceTypeLuaScript,
	}

	for _, resourceType := range resourceTypes {
		typeDir := filepath.Join(cacheDir, string(resourceType))
		if !util.Exists(typeDir) {
			continue
		}

		err := m.cleanResourceTypeDir(typeDir)
		if err != nil {
			return errors.Wrapf(err, "failed to clean cache for resource type: %s", resourceType)
		}
	}

	return nil
}

// cleanResourceTypeDir cleans cache entries for a specific resource type directory
func (m *DefaultResourceManager) cleanResourceTypeDir(typeDir string) error {
	entries, err := os.ReadDir(typeDir)
	if err != nil {
		return errors.Wrap(err, "failed to read resource type directory")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		resourceDir := filepath.Join(typeDir, entry.Name())
		err := m.cleanResourceDir(resourceDir)
		if err != nil {
			return errors.Wrapf(err, "failed to clean resource directory: %s", resourceDir)
		}
	}

	return nil
}

// cleanResourceDir cleans cache entries for a specific resource directory
func (m *DefaultResourceManager) cleanResourceDir(resourceDir string) error {
	versionEntries, err := os.ReadDir(resourceDir)
	if err != nil {
		return errors.Wrap(err, "failed to read resource directory")
	}

	for _, versionEntry := range versionEntries {
		if !versionEntry.IsDir() {
			continue
		}

		versionDir := filepath.Join(resourceDir, versionEntry.Name())
		err := m.cleanVersionDir(versionDir)
		if err != nil {
			return errors.Wrapf(err, "failed to clean version directory: %s", versionDir)
		}
	}

	// Remove empty resource directory if it has no versions left
	if isEmpty, _ := m.isDirEmpty(resourceDir); isEmpty {
		err := os.Remove(resourceDir)
		if err != nil {
			return errors.Wrapf(err, "failed to remove empty resource directory: %s", resourceDir)
		}
	}

	return nil
}

// cleanVersionDir cleans expired cache entries for a specific version directory
func (m *DefaultResourceManager) cleanVersionDir(versionDir string) error {
	hashEntries, err := os.ReadDir(versionDir)
	if err != nil {
		return errors.Wrap(err, "failed to read version directory")
	}

	cacheTTL := time.Hour * 24 * 7 // 1 week default TTL

	for _, hashEntry := range hashEntries {
		hashPath := filepath.Join(versionDir, hashEntry.Name())

		// Check if cache entry is expired
		info, err := hashEntry.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > cacheTTL {
			// Remove expired cache entry
			err := os.RemoveAll(hashPath)
			if err != nil {
				return errors.Wrapf(err, "failed to remove expired cache entry: %s", hashPath)
			}
		}
	}

	// Remove empty version directory if it has no hash entries left
	if isEmpty, _ := m.isDirEmpty(versionDir); isEmpty {
		err := os.Remove(versionDir)
		if err != nil {
			return errors.Wrapf(err, "failed to remove empty version directory: %s", versionDir)
		}
	}

	return nil
}

// isDirEmpty checks if a directory is empty
func (m *DefaultResourceManager) isDirEmpty(dirPath string) (bool, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
