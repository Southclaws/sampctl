package resource

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

// HTTPFileResource represents a single file downloaded from an HTTP URL.
// It caches the downloaded file under its original filename inside a stable
// cache directory (per version).
// Cache layout: <cacheDir>/<type>/<identifier>/<version>/<hash>/<filename>
// (where <hash> is derived from identifier+version).
type HTTPFileResource struct {
	baseResource *BaseResource
	url          string
	filename     string
}

func NewHTTPFileResource(rawURL, version string, resourceType ResourceType) (*HTTPFileResource, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL")
	}
	filename := filepath.Base(u.Path)
	if filename == "" || filename == "." || filename == "/" {
		return nil, errors.New("could not determine filename from URL")
	}

	identifier := filepath.Join("http", u.Host, filename)
	if version == "" {
		version = "latest"
	}

	hr := &HTTPFileResource{
		baseResource: NewBaseResource(identifier, version, resourceType),
		url:          rawURL,
		filename:     filename,
	}
	return hr, nil
}

// Version returns the resource version.
func (hr *HTTPFileResource) Version() string {
	return hr.baseResource.Version()
}

// Type returns the resource type.
func (hr *HTTPFileResource) Type() ResourceType {
	return hr.baseResource.Type()
}

// Identifier returns the unique resource identifier.
func (hr *HTTPFileResource) Identifier() string {
	if hr.baseResource == nil {
		return ""
	}
	return hr.baseResource.Identifier()
}

// SetCacheDir overrides the cache directory for this resource.
func (hr *HTTPFileResource) SetCacheDir(cacheDir string) {
	hr.baseResource.SetCacheDir(cacheDir)
}

// SetCacheTTL overrides the cache TTL for this resource.
func (hr *HTTPFileResource) SetCacheTTL(ttl time.Duration) {
	hr.baseResource.SetCacheTTL(ttl)
}

// SetLocalPath configures a local source path used for cache seeding.
func (hr *HTTPFileResource) SetLocalPath(path string) {
	hr.baseResource.SetLocalPath(path)
}

// EnsureFromLocal seeds the cache from a local file.
func (hr *HTTPFileResource) EnsureFromLocal(ctx context.Context, version, targetPath string) error {
	return hr.baseResource.EnsureFromLocal(ctx, version, targetPath)
}

func (hr *HTTPFileResource) Cached(version string) (bool, string) {
	cacheDir, err := hr.baseResource.cachePath(version)
	if err != nil {
		return false, ""
	}

	info, err := os.Stat(cacheDir)
	if err != nil {
		return false, ""
	}

	// Backward compatibility: older cache layout may have stored the file directly
	// at the hash path (a file, not a directory).
	if !info.IsDir() {
		if hr.baseResource.cacheTTL > 0 && time.Since(info.ModTime()) > hr.baseResource.cacheTTL {
			return false, ""
		}
		return true, cacheDir
	}

	cachedFile := filepath.Join(cacheDir, hr.filename)
	fileInfo, err := os.Stat(cachedFile)
	if err != nil {
		return false, ""
	}
	if hr.baseResource.cacheTTL > 0 && time.Since(fileInfo.ModTime()) > hr.baseResource.cacheTTL {
		return false, ""
	}

	return true, cachedFile
}

func (hr *HTTPFileResource) Ensure(ctx context.Context, version, path string) error {
	if version == "" {
		version = hr.baseResource.version
	}

	if cached, cachedPath := hr.Cached(version); cached {
		if err := hr.baseResource.MarkCached(cachedPath); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}
		if path != "" && path != cachedPath {
			return util.CopyFile(cachedPath, path)
		}
		return nil
	}

	cacheDirPath, err := hr.baseResource.cachePath(version)
	if err != nil {
		return err
	}

	// Ensure cache directory exists (remove legacy file if needed)
	if err := ensureCacheDir(cacheDirPath); err != nil {
		return err
	}
	destination := filepath.Join(cacheDirPath, hr.filename)
	if _, err := download.FromNet(ctx, hr.url, destination); err != nil {
		return errors.Wrap(err, "failed to download resource")
	}

	if path != "" && path != destination {
		if err := util.CopyFile(destination, path); err != nil {
			return errors.Wrap(err, "failed to copy to target path")
		}
	}

	return nil
}

func (hr *HTTPFileResource) GetURL() string {
	return hr.url
}

func (hr *HTTPFileResource) GetFilename() string {
	return hr.filename
}

// cacheKey exists only to keep the cache directory stable for a given identifier/version.
// It matches BaseResource's hashing scheme (identifier+":"+version) but is not exported.
func (hr *HTTPFileResource) cacheKey(version string) string {
	sum := md5.Sum([]byte(hr.Identifier() + ":" + version))
	return fmt.Sprintf("%x", sum[:8])
}
