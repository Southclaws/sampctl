package resource

import (
	"context"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

// HTTPResource represents a resource downloaded from an HTTP URL
type HTTPResource struct {
	*BaseResource
	url      string
	filename string
}

// NewHTTPResource creates a new HTTPResource
func NewHTTPResource(url, filename, version string, resourceType ResourceType) *HTTPResource {
	identifier := url
	if version == "" {
		version = "latest"
	}

	hr := &HTTPResource{
		BaseResource: NewBaseResource(identifier, version, resourceType),
		url:          url,
		filename:     filename,
	}
	
	hr.SetDownloadURL(url)
	return hr
}

// Ensure acquires the HTTP resource, downloading it if necessary
func (hr *HTTPResource) Ensure(ctx context.Context, version, path string) error {
	cachePath := hr.getCachePath(version)
	
	// Check if already cached
	if cached, cachedPath := hr.Cached(version); cached {
		// Mark as recently accessed
		if err := hr.MarkCached(cachedPath); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}
		
		// Copy to target path if different
		if path != "" && path != cachedPath {
			return util.CopyFile(cachedPath, path)
		}
		return nil
	}
	
	// Ensure cache directory exists
	if err := hr.ensureCacheDir(cachePath); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}
	
	// Download the file
	filename := hr.filename
	if filename == "" {
		filename = filepath.Base(hr.url)
	}
	
	_, err := download.FromNet(hr.url, filepath.Dir(cachePath), filename)
	if err != nil {
		return errors.Wrap(err, "failed to download HTTP resource")
	}
	
	// Copy to target path if specified
	if path != "" && path != cachePath {
		if err := util.CopyFile(cachePath, path); err != nil {
			return errors.Wrap(err, "failed to copy to target path")
		}
	}
	
	return nil
}

// GetURL returns the download URL
func (hr *HTTPResource) GetURL() string {
	return hr.url
}

// GetFilename returns the filename
func (hr *HTTPResource) GetFilename() string {
	return hr.filename
}
