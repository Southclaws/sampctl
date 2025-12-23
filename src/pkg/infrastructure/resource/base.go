package resource

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

// BaseResource provides common functionality for all resource types
type BaseResource struct {
	identifier   string
	version      string
	resourceType ResourceType
	cacheDir     string
	downloadURL  string
	localPath    string
	cacheTTL     time.Duration
}

// NewBaseResource creates a new BaseResource
func NewBaseResource(identifier, version string, resourceType ResourceType) *BaseResource {
	return &BaseResource{
		identifier:   identifier,
		version:      version,
		resourceType: resourceType,
		cacheDir:     fs.MustConfigDir(),
		cacheTTL:     time.Hour * 24 * 7, // Default 1 week cache
	}
}

// Version returns the resource version
func (br *BaseResource) Version() string {
	return br.version
}

// Type returns the resource type
func (br *BaseResource) Type() ResourceType {
	return br.resourceType
}

// Identifier returns the unique identifier
func (br *BaseResource) Identifier() string {
	return br.identifier
}

// Cached checks if the resource is cached and returns the path if present
func (br *BaseResource) Cached(version string) (bool, string) {
	cachePath := br.getCachePath(version)

	// Check if cached file/directory exists
	if !fs.Exists(cachePath) {
		return false, ""
	}

	// Check if cache is still valid (not expired)
	info, err := os.Stat(cachePath)
	if err != nil {
		return false, ""
	}

	if time.Since(info.ModTime()) > br.cacheTTL {
		return false, ""
	}

	return true, cachePath
}

// getCachePath returns the cache path for a specific version
func (br *BaseResource) getCachePath(version string) string {
	// Create a hash of the identifier + version for unique cache paths
	sum := md5.Sum([]byte(br.identifier + ":" + version))

	return filepath.Join(
		br.cacheDir,
		string(br.resourceType),
		br.identifier,
		version,
		fmt.Sprintf("%x", sum[:8]),
	)
}

// ensureCacheDir creates the cache directory if it doesn't exist
func (br *BaseResource) ensureCacheDir(cachePath string) error {
	dir := filepath.Dir(cachePath)
	return fs.EnsureDir(dir, fs.PermDirPrivate)
}

// SetCacheDir allows overriding the default cache directory
func (br *BaseResource) SetCacheDir(cacheDir string) {
	br.cacheDir = cacheDir
}

// SetCacheTTL allows overriding the default cache TTL
func (br *BaseResource) SetCacheTTL(ttl time.Duration) {
	br.cacheTTL = ttl
}

// SetDownloadURL sets the download URL for this resource
func (br *BaseResource) SetDownloadURL(url string) {
	br.downloadURL = url
}

// SetLocalPath sets a local path for this resource
func (br *BaseResource) SetLocalPath(path string) {
	br.localPath = path
}

// GetDownloadURL returns the download URL
func (br *BaseResource) GetDownloadURL() string {
	return br.downloadURL
}

// GetLocalPath returns the local path
func (br *BaseResource) GetLocalPath() string {
	return br.localPath
}

// MarkCached updates the cache timestamp to mark the resource as recently accessed
func (br *BaseResource) MarkCached(cachePath string) error {
	now := time.Now()
	return os.Chtimes(cachePath, now, now)
}

// EnsureFromLocal copies a local file/directory to the cache
func (br *BaseResource) EnsureFromLocal(ctx context.Context, version, targetPath string) error {
	if br.localPath == "" {
		return errors.New("no local path specified for resource")
	}

	cachePath := br.getCachePath(version)

	// Check if already cached
	if cached, path := br.Cached(version); cached {
		// Copy from cache to target if different
		if path != targetPath && targetPath != "" {
			return util.CopyFile(path, targetPath)
		}
		return nil
	}

	// Ensure cache directory exists
	if err := br.ensureCacheDir(cachePath); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}

	// Copy local file to cache
	if err := util.CopyFile(br.localPath, cachePath); err != nil {
		return errors.Wrap(err, "failed to copy local file to cache")
	}

	// Copy to target path if specified
	if targetPath != "" && targetPath != cachePath {
		if err := util.CopyFile(cachePath, targetPath); err != nil {
			return errors.Wrap(err, "failed to copy to target path")
		}
	}

	return nil
}

// EnsureFromURL downloads a file from URL to the cache
func (br *BaseResource) EnsureFromURL(ctx context.Context, version, targetPath string) error {
	if br.downloadURL == "" {
		return errors.New("no download URL specified for resource")
	}

	cachePath := br.getCachePath(version)

	// Check if already cached
	if cached, path := br.Cached(version); cached {
		if err := br.MarkCached(path); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}
		// Copy from cache to target if different
		if path != targetPath && targetPath != "" {
			return util.CopyFile(path, targetPath)
		}
		return nil
	}

	// Ensure cache directory exists
	if err := br.ensureCacheDir(cachePath); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}

	_, err := download.FromNet(ctx, br.downloadURL, cachePath)
	if err != nil {
		return errors.Wrap(err, "failed to download resource")
	}

	// Copy to target path if specified
	if targetPath != "" && targetPath != cachePath {
		if err := util.CopyFile(cachePath, targetPath); err != nil {
			return errors.Wrap(err, "failed to copy to target path")
		}
	}

	return nil
}
