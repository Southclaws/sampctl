package resource

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

// Cached checks if the resource is cached and returns the cached file path if present.
// Unlike BaseResource, GitHubReleaseResource stores the downloaded asset under its
// original filename inside a stable cache directory.
func (ghr *GitHubReleaseResource) Cached(version string) (bool, string) {
	cacheDir := ghr.getCachePath(version)

	info, err := os.Stat(cacheDir)
	if err != nil {
		return false, ""
	}

	// Backward compatibility: older cache layout stored the downloaded asset directly
	// at the hash path
	if !info.IsDir() {
		if ghr.cacheTTL > 0 && time.Since(info.ModTime()) > ghr.cacheTTL {
			return false, ""
		}
		return true, cacheDir
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return false, ""
	}

	var cachedFile string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		cachedFile = filepath.Join(cacheDir, entry.Name())
		break
	}
	if cachedFile == "" {
		return false, ""
	}

	fileInfo, err := os.Stat(cachedFile)
	if err != nil {
		return false, ""
	}
	if ghr.cacheTTL > 0 && time.Since(fileInfo.ModTime()) > ghr.cacheTTL {
		return false, ""
	}

	return true, cachedFile
}

// NewGitHubReleaseResource creates a new GitHubReleaseResource
func NewGitHubReleaseResource(
	depMeta versioning.DependencyMeta,
	assetPattern *regexp.Regexp,
	resourceType ResourceType,
	ghClient *github.Client,
) *GitHubReleaseResource {
	identifier := identifierFromDependencyMeta(depMeta)
	if assetPattern != nil {
		sum := md5.Sum([]byte(assetPattern.String()))
		identifier = fmt.Sprintf("%s-%x", identifier, sum[:4])
	}
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
	if version == "" {
		version = ghr.version
	}
	if ghr.ghClient == nil {
		return errors.New("no GitHub client provided")
	}

	cacheDirPath := ghr.getCachePath(version)

	// Check if already cached
	if cached, cachedPath := ghr.Cached(version); cached {
		if err := ghr.MarkCached(cachedPath); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}
		if path != "" && path != cachedPath {
			return ghr.copyToTarget(cachedPath, path)
		}
		return nil
	}

	if err := ensureCacheDir(cacheDirPath); err != nil {
		return err
	}

	meta := ghr.dependencyMeta
	if version == "latest" {
		meta.Tag = ""
	} else {
		meta.Tag = version
	}

	downloadDir := cacheDirPath
	if ghr.cacheDir != "" {
		if rel, relErr := filepath.Rel(ghr.cacheDir, downloadDir); relErr == nil && rel != "" && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, "..") {
			downloadDir = rel
		}
	}

	filename, tag, err := download.ReleaseAssetByPattern(
		ctx,
		ghr.ghClient,
		meta,
		ghr.assetPattern,
		downloadDir,
		"",
		ghr.cacheDir,
	)
	if err != nil {
		return errors.Wrap(err, "failed to download GitHub release asset")
	}

	resolvedVersion := version
	if resolvedVersion == "latest" && tag != "" {
		resolvedVersion = tag
		ghr.version = tag
	}

	// If we resolved "latest" to an actual tag, move the downloaded asset to the tagged cache path.
	if resolvedVersion != version {
		newCacheDirPath := ghr.getCachePath(resolvedVersion)
		if err := ensureCacheDir(newCacheDirPath); err != nil {
			return err
		}
		newFilename := filepath.Join(newCacheDirPath, filepath.Base(filename))
		if filename != newFilename {
			if err := os.Rename(filename, newFilename); err != nil {
				if copyErr := util.CopyFile(filename, newFilename); copyErr != nil {
					return errors.Wrap(err, "failed to move downloaded asset")
				}
				_ = os.Remove(filename)
			}
			filename = newFilename
		}
		cacheDirPath = newCacheDirPath
	}

	if ghr.extractFunc != nil && ghr.extractPaths != nil {
		extractedFiles, err := ghr.extractFunc(filename, filepath.Dir(filename), ghr.extractPaths)
		if err != nil {
			return errors.Wrap(err, "failed to extract archive")
		}
		if path != "" {
			for _, extractedFile := range extractedFiles {
				relPath, err := filepath.Rel(filepath.Dir(filename), extractedFile)
				if err != nil {
					continue
				}
				targetFile := filepath.Join(path, relPath)
				if err := ghr.copyToTarget(extractedFile, targetFile); err != nil {
					return errors.Wrapf(err, "failed to copy extracted file %s to target", extractedFile)
				}
			}
		}
		return nil
	}

	if path != "" && path != filename {
		if err := ghr.copyToTarget(filename, path); err != nil {
			return errors.Wrap(err, "failed to copy to target path")
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
