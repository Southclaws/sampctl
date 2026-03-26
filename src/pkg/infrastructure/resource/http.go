package resource

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// HTTPResource represents a resource downloaded from an HTTP URL
type HTTPResource struct {
	baseResource *BaseResource
	url          string
	filename     string
}

// NewHTTPResource creates a new HTTPResource
func NewHTTPResource(url, filename, version string, resourceType ResourceType) *HTTPResource {
	identifier := url
	if version == "" {
		version = "latest"
	}

	hr := &HTTPResource{
		baseResource: NewBaseResource(identifier, version, resourceType),
		url:          url,
		filename:     filename,
	}

	hr.baseResource.SetDownloadURL(url)
	return hr
}

// Version returns the resource version.
func (hr *HTTPResource) Version() string {
	return hr.baseResource.Version()
}

// Type returns the resource type.
func (hr *HTTPResource) Type() ResourceType {
	return hr.baseResource.Type()
}

// Identifier returns the unique resource identifier.
func (hr *HTTPResource) Identifier() string {
	return hr.baseResource.Identifier()
}

// Cached reports whether the resource is already cached.
func (hr *HTTPResource) Cached(version string) (bool, string) {
	return hr.baseResource.Cached(version)
}

// SetCacheDir overrides the cache directory for this resource.
func (hr *HTTPResource) SetCacheDir(cacheDir string) {
	hr.baseResource.SetCacheDir(cacheDir)
}

// SetCacheTTL overrides the cache TTL for this resource.
func (hr *HTTPResource) SetCacheTTL(ttl time.Duration) {
	hr.baseResource.SetCacheTTL(ttl)
}

// Ensure acquires the HTTP resource, downloading it if necessary
func (hr *HTTPResource) Ensure(ctx context.Context, version, path string) error {
	if hr.baseResource == nil {
		return errors.New("HTTPResource has no BaseResource")
	}
	hr.baseResource.SetDownloadURL(hr.url)
	return hr.baseResource.EnsureFromURL(ctx, version, path)
}

// GetURL returns the download URL
func (hr *HTTPResource) GetURL() string {
	return hr.url
}

// GetFilename returns the filename
func (hr *HTTPResource) GetFilename() string {
	return hr.filename
}
