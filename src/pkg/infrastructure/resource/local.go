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
	baseResource *BaseResource
	localPath    string
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
		baseResource: NewBaseResource(identifier, version, resourceType),
		localPath:    localPath,
	}

	lr.baseResource.SetLocalPath(localPath)

	return lr
}

// Version returns the resource version.
func (lr *LocalResource) Version() string {
	return lr.baseResource.Version()
}

// Type returns the resource type.
func (lr *LocalResource) Type() ResourceType {
	return lr.baseResource.Type()
}

// Identifier returns the unique resource identifier.
func (lr *LocalResource) Identifier() string {
	return lr.baseResource.Identifier()
}

// Ensure acquires the local resource, copying it if necessary
func (lr *LocalResource) Ensure(_ context.Context, _ string, path string) error {
	if path == "" {
		// With no target, the resource remains at its original path.
		return nil
	}

	if _, err := os.Stat(lr.localPath); err != nil {
		return errors.Wrapf(err, "local resource not found: %s", lr.localPath)
	}

	if err := util.CopyFile(lr.localPath, path); err != nil {
		return errors.Wrap(err, "failed to copy local resource to target path")
	}

	return nil
}

// Cached returns the original path when the local resource exists.
func (lr *LocalResource) Cached(_ string) (bool, string) {
	if _, err := os.Stat(lr.localPath); err == nil {
		return true, lr.localPath
	}
	return false, ""
}

// GetLocalPath returns the local path
func (lr *LocalResource) GetLocalPath() string {
	return lr.localPath
}
