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
	*BaseResource
	url      string
	filename string
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
		BaseResource: NewBaseResource(identifier, version, resourceType),
		url:          rawURL,
		filename:     filename,
	}
	return hr, nil
}

func (hr *HTTPFileResource) Cached(version string) (bool, string) {
	cacheDir := hr.getCachePath(version)

	info, err := os.Stat(cacheDir)
	if err != nil {
		return false, ""
	}

	// Backward compatibility: older cache layout may have stored the file directly
	// at the hash path (a file, not a directory).
	if !info.IsDir() {
		if hr.cacheTTL > 0 && time.Since(info.ModTime()) > hr.cacheTTL {
			return false, ""
		}
		return true, cacheDir
	}

	cachedFile := filepath.Join(cacheDir, hr.filename)
	fileInfo, err := os.Stat(cachedFile)
	if err != nil {
		return false, ""
	}
	if hr.cacheTTL > 0 && time.Since(fileInfo.ModTime()) > hr.cacheTTL {
		return false, ""
	}

	return true, cachedFile
}

func (hr *HTTPFileResource) Ensure(ctx context.Context, version, path string) error {
	if version == "" {
		version = hr.version
	}

	if cached, cachedPath := hr.Cached(version); cached {
		if err := hr.MarkCached(cachedPath); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}
		if path != "" && path != cachedPath {
			return util.CopyFile(cachedPath, path)
		}
		return nil
	}

	cacheDirPath := hr.getCachePath(version)

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

// Identifier returns the stable identifier used for caching.
func (hr *HTTPFileResource) Identifier() string {
	if hr.BaseResource == nil {
		return ""
	}
	return hr.BaseResource.Identifier()
}

func (hr *HTTPFileResource) Type() ResourceType {
	return hr.BaseResource.Type()
}

func (hr *HTTPFileResource) Version() string {
	return hr.BaseResource.Version()
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
