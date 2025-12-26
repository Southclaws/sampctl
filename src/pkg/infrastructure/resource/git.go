package resource

import (
	"context"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// GitResource represents a resource from a Git repository (GitHub, GitLab, etc.)
type GitResource struct {
	*BaseResource
	dependencyMeta versioning.DependencyMeta
}

// NewGitResource creates a new GitResource from a dependency meta
func NewGitResource(depMeta versioning.DependencyMeta, resourceType ResourceType) *GitResource {
	identifier := identifierFromDependencyMeta(depMeta)
	version := depMeta.Tag
	if version == "" {
		version = depMeta.Branch
	}
	if version == "" {
		version = depMeta.Commit
	}
	if version == "" {
		version = "latest"
	}

	return &GitResource{
		BaseResource:   NewBaseResource(identifier, version, resourceType),
		dependencyMeta: depMeta,
	}
}

// Ensure acquires the Git resource, cloning/updating the repository as needed
func (gr *GitResource) Ensure(ctx context.Context, version, path string) error {
	cachePath := gr.getCachePath(version)

	// Check if already cached
	if cached, cachedPath := gr.Cached(version); cached {
		// Mark as recently accessed
		if err := gr.MarkCached(cachedPath); err != nil {
			return errors.Wrap(err, "failed to update cache timestamp")
		}

		// Copy/link to target path if different
		if path != "" && path != cachedPath {
			return gr.copyToTarget(cachedPath, path)
		}
		return nil
	}

	// Ensure cache directory exists
	if err := gr.ensureCacheDir(cachePath); err != nil {
		return errors.Wrap(err, "failed to create cache directory")
	}

	meta := gr.dependencyMeta
	if meta.Site == "" {
		meta.Site = "github.com"
	}

	// If the path exists (potentially because cache TTL expired), remove it so clone succeeds.
	_ = os.RemoveAll(cachePath)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return errors.Wrap(err, "failed to create parent cache directory")
	}

	cloneOpts := &git.CloneOptions{
		URL: meta.URL(),
	}

	if meta.Tag != "" {
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(meta.Tag)
		cloneOpts.SingleBranch = true
	} else if meta.Branch != "" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(meta.Branch)
		cloneOpts.SingleBranch = true
	}

	repo, err := git.PlainCloneContext(ctx, cachePath, false, cloneOpts)
	if err != nil {
		return errors.Wrap(err, "failed to clone git dependency")
	}

	if meta.Commit != "" {
		wt, werr := repo.Worktree()
		if werr != nil {
			return errors.Wrap(werr, "failed to get git worktree")
		}
		if werr := wt.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(meta.Commit)}); werr != nil {
			return errors.Wrap(werr, "failed to checkout commit")
		}
	}

	// Copy to target path if specified
	if path != "" && path != cachePath {
		if err := gr.copyToTarget(cachePath, path); err != nil {
			return errors.Wrap(err, "failed to copy to target path")
		}
	}

	return nil
}

// copyToTarget handles copying/linking files or directories to target location
func (gr *GitResource) copyToTarget(source, target string) error {
	// For now, use simple copy operation
	// In the future, this could be optimized with hard links or symlinks
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(target, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		return util.CopyFile(path, targetPath)
	})
}

// GetDependencyMeta returns the underlying dependency metadata
func (gr *GitResource) GetDependencyMeta() versioning.DependencyMeta {
	return gr.dependencyMeta
}
