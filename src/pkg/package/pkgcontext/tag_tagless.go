package pkgcontext

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

var ErrDependencyHasNoTagsOrReleases = errors.New("dependency does not publish tags or releases")

// TagTaglessDependencies preserves the legacy helper name for updating direct
// dependency references during an ensure/update flow.
func (pcx *PackageContext) TagTaglessDependencies(ctx context.Context, forceUpdate bool) (bool, error) {
	return pcx.UpdateDependencyReferences(ctx, DependencyUpdateRequest{
		Enabled: true,
		Force:   forceUpdate,
	})
}

func (pcx *PackageContext) readDefinitionSnapshot() (path string, perm os.FileMode, contents []byte, err error) {
	var name string
	switch pcx.Package.Format {
	case "json":
		name = "pawn.json"
	case "yaml":
		name = "pawn.yaml"
	default:
		err = errors.New("package definition format must be json or yaml")
		return
	}

	path = filepath.Join(pcx.Package.LocalPath, name)
	stat, statErr := os.Stat(path)
	if statErr != nil {
		err = statErr
		return
	}
	perm = stat.Mode().Perm()

	contents, err = os.ReadFile(path)
	if err != nil {
		return
	}
	return
}

func formatPinnedDependency(meta versioning.DependencyMeta) string {
	if meta.IsURLScheme() {
		return meta.String()
	}

	base := meta.User + "/" + meta.Repo
	if meta.Path != "" {
		base += "/" + strings.TrimPrefix(meta.Path, "/")
	}

	if meta.Tag != "" {
		return base + ":" + meta.Tag
	}
	if meta.Branch != "" {
		return base + "@" + meta.Branch
	}
	if meta.Commit != "" {
		return base + "#" + meta.Commit
	}

	return base
}

func (pcx *PackageContext) resolveLatestTag(ctx context.Context, meta versioning.DependencyMeta, forceUpdate bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if forceUpdate {
		tag, err := pcx.resolveFreshLatestTag(ctx, meta)
		if err == nil && tag != "" {
			return tag, nil
		}
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			if isMissingLatestReleaseError(err) {
				return "", ErrDependencyHasNoTagsOrReleases
			}
			print.Verb(meta, "failed to resolve fresh latest tag")
		}
	}

	tag, err := pcx.latestTagFromCacheContext(ctx, meta)
	if err == nil && tag != "" {
		return tag, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrInvalid) || errors.Is(err, os.ErrPermission) {
		return "", err
	}

	if pcx.GitHub != nil && (meta.Site == "" || meta.Site == "github.com") {
		tag, err := pcx.latestTagFromGitHubRelease(ctx, meta)
		if err == nil && tag != "" {
			return tag, nil
		}
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			print.Verb(meta, "failed to get latest release tag")
		}
	}

	refreshCache := forceUpdate
	if refreshCache {
		noOrigin, err := pcx.cachedRepoHasNoOriginContext(ctx, meta)
		if err != nil {
			return "", err
		}
		refreshCache = !noOrigin
	}
	_, ensureErr := pcx.EnsureDependencyCachedContext(ctx, meta, refreshCache)
	if ensureErr != nil {
		return "", errors.Wrap(ensureErr, "failed to refresh dependency cache")
	}

	tag, err = pcx.latestTagFromCacheContext(ctx, meta)
	if err != nil {
		if isMissingLatestReleaseError(err) {
			return "", ErrDependencyHasNoTagsOrReleases
		}
		return "", errors.Wrap(err, "failed to read tags after refreshing dependency cache")
	}
	return tag, nil
}

func (pcx *PackageContext) resolveFreshLatestTag(ctx context.Context, meta versioning.DependencyMeta) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if pcx.GitHub != nil && (meta.Site == "" || meta.Site == "github.com") {
		tag, err := pcx.latestTagFromGitHubRelease(ctx, meta)
		if err == nil && tag != "" {
			return tag, nil
		}
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			print.Verb(meta, "failed to get latest release tag")
		}
	}

	noOrigin, err := pcx.cachedRepoHasNoOriginContext(ctx, meta)
	if err != nil {
		return "", err
	}
	if noOrigin {
		return pcx.latestTagFromCacheContext(ctx, meta)
	}

	if _, err := pcx.EnsureDependencyCachedContext(ctx, meta, true); err != nil {
		return "", errors.Wrap(err, "failed to refresh dependency cache")
	}

	tag, err := pcx.latestTagFromCacheContext(ctx, meta)
	if err != nil {
		if isMissingLatestReleaseError(err) {
			return "", ErrDependencyHasNoTagsOrReleases
		}
		return "", errors.Wrap(err, "failed to read tags after refreshing dependency cache")
	}

	return tag, nil
}

func isMissingLatestReleaseError(err error) bool {
	switch errors.Cause(err) {
	case ErrDependencyHasNoTagsOrReleases:
		return true
	}

	switch errors.Cause(err).Error() {
	case "no tags", "no releases", "no usable release tag":
		return true
	default:
		return false
	}
}

func (pcx *PackageContext) cachedRepoHasNoOriginContext(ctx context.Context, meta versioning.DependencyMeta) (noOrigin bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	repoPath, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		return false, err
	}
	unlock, err := lockRepositoryPath(ctx, repoPath)
	if err != nil {
		return false, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()
	if err := validateRepositoryDestination(repoPath); err != nil {
		return false, err
	}
	repo, err := pcx.PackageServices.repositoryStore().Open(repoPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, nil
	}
	remote, err := repo.Remote("origin")
	if errors.Is(err, git.ErrRemoteNotFound) {
		return true, nil
	}
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, nil
	}
	return remote == nil || remote.Config() == nil || len(remote.Config().URLs) == 0 || remote.Config().URLs[0] == "", nil
}

func (pcx *PackageContext) latestTagFromGitHubRelease(ctx context.Context, meta versioning.DependencyMeta) (string, error) {
	releases, _, err := pcx.GitHub.Repositories.ListReleases(ctx, meta.User, meta.Repo, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to list releases")
	}
	if len(releases) == 0 {
		return "", errors.New("no releases")
	}

	for _, r := range releases {
		if r.GetDraft() {
			continue
		}
		if r.GetPrerelease() {
			continue
		}
		if r.GetTagName() != "" {
			return r.GetTagName(), nil
		}
	}
	for _, r := range releases {
		if r.GetDraft() {
			continue
		}
		if r.GetTagName() != "" {
			return r.GetTagName(), nil
		}
	}

	return "", errors.New("no usable release tag")
}

func (pcx *PackageContext) latestTagFromCache(meta versioning.DependencyMeta) (string, error) {
	return pcx.latestTagFromCacheContext(context.Background(), meta)
}

func (pcx *PackageContext) latestTagFromCacheContext(ctx context.Context, meta versioning.DependencyMeta) (tag string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cachePath, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		return "", err
	}
	unlock, err := lockRepositoryPath(ctx, cachePath)
	if err != nil {
		return "", err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()
	if err := validateRepositoryDestination(cachePath); err != nil {
		return "", err
	}
	gitPath := filepath.Join(cachePath, ".git")
	if !fs.Exists(gitPath) {
		return "", errors.New("no cached repository")
	}

	repo, err := git.PlainOpen(cachePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to open cached repository")
	}

	versionedTags, err := versioning.GetRepoSemverTags(repo)
	if err == nil && len(versionedTags) > 0 {
		latest := versionedTags[0]
		for _, vt := range versionedTags[1:] {
			if latest.Version == nil {
				latest = vt
				continue
			}
			if vt.Version != nil && vt.Version.GreaterThan(latest.Version) {
				latest = vt
			}
		}
		if latest.Name != "" {
			return latest.Name, nil
		}
	}

	tags, err := repo.Tags()
	if err != nil {
		return "", errors.Wrap(err, "failed to list tags")
	}
	defer tags.Close()

	var (
		bestName string
		bestTime time.Time
		bestVer  *semver.Version
	)

	err = tags.ForEach(func(pr *plumbing.Reference) error {
		ref, errInner := versioning.RefFromTagRef(repo, pr)
		if errInner != nil {
			return nil
		}
		commit, errInner := repo.CommitObject(ref.Hash())
		if errInner != nil {
			return nil
		}

		tagName := pr.Name().Short()
		when := commit.Committer.When

		var parsed *semver.Version
		if v, vErr := semver.NewVersion(tagName); vErr == nil {
			parsed = v
		}

		if bestName == "" || when.After(bestTime) {
			bestName = tagName
			bestTime = when
			bestVer = parsed
			return nil
		}
		if when.Equal(bestTime) {
			if parsed != nil && bestVer != nil {
				if parsed.GreaterThan(bestVer) {
					bestName = tagName
					bestVer = parsed
				}
			} else if bestVer == nil && parsed != nil {
				bestName = tagName
				bestVer = parsed
			} else if bestVer == nil && parsed == nil {
				// Deterministic tie-break for non-semver tags: prefer the lexicographically
				// greatest tag name (stable across map/iterator ordering).
				if tagName > bestName {
					bestName = tagName
				}
			}
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, storer.ErrStop) {
			return bestName, nil
		}
		return "", errors.Wrap(err, "failed to iterate tags")
	}

	if bestName == "" {
		return "", errors.New("no tags")
	}
	return bestName, nil
}
