package resource

import (
	"context"

	"github.com/pkg/errors"
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
	if hr.BaseResource == nil {
		return errors.New("HTTPResource has no BaseResource")
	}
	hr.SetDownloadURL(hr.url)
	return hr.EnsureFromURL(ctx, version, path)
}

// GetURL returns the download URL
func (hr *HTTPResource) GetURL() string {
	return hr.url
}

// GetFilename returns the filename
func (hr *HTTPResource) GetFilename() string {
	return hr.filename
}
