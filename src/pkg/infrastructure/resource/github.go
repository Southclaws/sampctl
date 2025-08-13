package resource

import (
	"context"
	"path/filepath"
	"regexp"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// GitHubReleaseResource represents a resource from a GitHub release asset
type GitHubReleaseResource struct {
	*BaseResource
	dependencyMeta versioning.DependencyMeta
	assetPattern   *regexp.Regexp
	ghClient       *github.Client
	extractFunc    download.ExtractFunc
	extractPaths   map[string]string
}

// NewGitHubReleaseResource creates a new GitHubReleaseResource
func NewGitHubReleaseResource(
	depMeta versioning.DependencyMeta,
	assetPattern *regexp.Regexp,
	resourceType ResourceType,
	ghClient *github.Client,
) *GitHubReleaseResource {
	identifier := depMeta.String()
	version := depMeta.Tag
	if version == "" {
		version = "latest"
	}

	return &GitHubReleaseResource{
		BaseResource:   NewBaseResource(identifier, version, resourceType),
		dependencyMeta: depMeta,
		assetPattern:   assetPattern,
		ghClient:       ghClient,
	}
}

// SetExtractFunc sets the extraction function for archive assets
func (ghr *GitHubReleaseResource) SetExtractFunc(extractFunc download.ExtractFunc) {
	ghr.extractFunc = extractFunc
}

// SetExtractPaths sets the extraction paths for archive assets
func (ghr *GitHubReleaseResource) SetExtractPaths(paths map[string]string) {
	ghr.extractPaths = paths
}

// Ensure acquires the GitHub release asset, downloading and extracting as needed
func (ghr *GitHubReleaseResource) Ensure(ctx context.Context, version, path string) error {
	cachePath := ghr.getCachePath(version)
	
	// Check if already cached
	if cached, cachedPath := ghr.Cached(version); cached {
		// Mark as recently accessed
		if err := ghr.MarkCached(cachedPath); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}
		
		// Copy to target path if different
		if path != "" && path != cachedPath {
			return ghr.copyToTarget(cachedPath, path)
		}
		return nil
	}
	
	// Ensure cache directory exists
	if err := ghr.ensureCacheDir(cachePath); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}
	
	// Download the release asset
	filename, tag, err := download.ReleaseAssetByPattern(
		ctx,
		ghr.ghClient,
		ghr.dependencyMeta,
		ghr.assetPattern,
		filepath.Dir(cachePath),
		filepath.Base(cachePath),
		ghr.cacheDir,
	)
	if err != nil {
		return errors.Wrap(err, "failed to download GitHub release asset")
	}
	
	// Update version if we got latest
	if ghr.version == "latest" && tag != "" {
		ghr.version = tag
		// Recalculate cache path with actual version
		cachePath = ghr.getCachePath(ghr.version)
	}
	
	// If it's an archive and we have extraction settings, extract it
	if ghr.extractFunc != nil && ghr.extractPaths != nil {
		extractedFiles, err := ghr.extractFunc(filename, filepath.Dir(cachePath), ghr.extractPaths)
		if err != nil {
			return errors.Wrap(err, "failed to extract archive")
		}
		
		// The extracted files are now in the cache directory
		// If we have a target path, copy the extracted files there
		if path != "" {
			for _, extractedFile := range extractedFiles {
				relPath, err := filepath.Rel(filepath.Dir(cachePath), extractedFile)
				if err != nil {
					continue
				}
				targetFile := filepath.Join(path, relPath)
				if err := ghr.copyToTarget(extractedFile, targetFile); err != nil {
					return errors.Wrapf(err, "failed to copy extracted file %s to target", extractedFile)
				}
			}
		}
	} else {
		// Single file, copy to target if specified
		if path != "" && path != cachePath {
			if err := ghr.copyToTarget(cachePath, path); err != nil {
				return errors.Wrap(err, "failed to copy to target path")
			}
		}
	}
	
	return nil
}

// copyToTarget handles copying files to target location
func (ghr *GitHubReleaseResource) copyToTarget(source, target string) error {
	return util.CopyFile(source, target)
}

// GetDependencyMeta returns the underlying dependency metadata
func (ghr *GitHubReleaseResource) GetDependencyMeta() versioning.DependencyMeta {
	return ghr.dependencyMeta
}

// GetAssetPattern returns the asset pattern used for matching
func (ghr *GitHubReleaseResource) GetAssetPattern() *regexp.Regexp {
	return ghr.assetPattern
}
