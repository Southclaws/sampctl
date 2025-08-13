package resource

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

// LocalResource represents a resource from a local file or directory
type LocalResource struct {
	*BaseResource
	localPath string
}

// NewLocalResource creates a new LocalResource
func NewLocalResource(localPath string, resourceType ResourceType) *LocalResource {
	identifier := localPath
	
	// For local resources, use the file modification time as version
	version := "local"
	if stat, err := os.Stat(localPath); err == nil {
		version = stat.ModTime().Format(time.RFC3339)
	}

	lr := &LocalResource{
		BaseResource: NewBaseResource(identifier, version, resourceType),
		localPath:    localPath,
	}
	
	lr.SetLocalPath(localPath)
	return lr
}

// Ensure acquires the local resource, copying it if necessary
func (lr *LocalResource) Ensure(ctx context.Context, version, path string) error {
	// For local resources, we can just copy directly without caching
	// unless a cache is specifically requested
	
	if path == "" {
		// No target path specified, resource is already "ensured" at its local path
		return nil
	}
	
	// Check if source exists
	if _, err := os.Stat(lr.localPath); err != nil {
		return errors.Wrapf(err, "local resource not found: %s", lr.localPath)
	}
	
	// Copy to target path
	if err := util.CopyFile(lr.localPath, path); err != nil {
		return errors.Wrap(err, "failed to copy local resource to target path")
	}
	
	return nil
}

// Cached always returns true for local resources since they don't need caching
func (lr *LocalResource) Cached(version string) (bool, string) {
	// Local resources are always "cached" at their original location
	if _, err := os.Stat(lr.localPath); err == nil {
		return true, lr.localPath
	}
	return false, ""
}

// GetLocalPath returns the local path
func (lr *LocalResource) GetLocalPath() string {
	return lr.localPath
}
