package pkgcontext

import (
	"context"
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

// TagTaglessDependencies updates the root package definition file so any dependency
// without an explicit tag/branch/commit gets pinned to the latest available tag.
func (pcx *PackageContext) TagTaglessDependencies(ctx context.Context, forceUpdate bool) (bool, error) {
	if !pcx.Package.Parent {
		return false, nil
	}
	if pcx.Package.LocalPath == "" {
		return false, errors.New("package has no local path")
	}

	changedDeps, err := pcx.tagTaglessDependencyList(ctx, pcx.Package.Dependencies, forceUpdate)
	if err != nil {
		return false, err
	}
	changedDev, err := pcx.tagTaglessDependencyList(ctx, pcx.Package.Development, forceUpdate)
	if err != nil {
		return false, err
	}

	changed := changedDeps.changed || changedDev.changed
	if !changed {
		return false, nil
	}

	pcx.Package.Dependencies = changedDeps.updated
	pcx.Package.Development = changedDev.updated

	if err := pcx.Package.WriteDefinition(); err != nil {
		return false, errors.Wrap(err, "failed to write updated package definition")
	}

	if err := pcx.EnsureDependenciesCached(); err != nil {
		return true, errors.Wrap(err, "failed to refresh dependency tree after tagging")
	}

	return true, nil
}

type tagListResult struct {
	updated []versioning.DependencyString
	changed bool
}

func (pcx *PackageContext) tagTaglessDependencyList(ctx context.Context, deps []versioning.DependencyString, forceUpdate bool) (tagListResult, error) {
	res := tagListResult{updated: make([]versioning.DependencyString, 0, len(deps))}
	for _, depStr := range deps {
		meta, err := depStr.Explode()
		if err != nil {
			res.updated = append(res.updated, depStr)
			continue
		}

		if meta.IsLocalScheme() {
			res.updated = append(res.updated, depStr)
			continue
		}
		if meta.Tag != "" || meta.Branch != "" || meta.Commit != "" {
			res.updated = append(res.updated, depStr)
			continue
		}
		if meta.User == "" || meta.Repo == "" {
			res.updated = append(res.updated, depStr)
			continue
		}

		tag, err := pcx.resolveLatestTag(ctx, meta, forceUpdate)
		if err != nil {
			print.Verb(meta, "failed to resolve latest tag:", err)
			res.updated = append(res.updated, depStr)
			continue
		}
		if tag == "" {
			res.updated = append(res.updated, depStr)
			continue
		}

		meta.Tag = tag
		newStr := versioning.DependencyString(formatPinnedDependency(meta))
		res.updated = append(res.updated, newStr)
		if newStr != depStr {
			res.changed = true
			print.Verb("tagged dependency", depStr, "->", newStr)
		}
	}
	return res, nil
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
	tag, err := pcx.latestTagFromCache(meta)
	if err == nil && tag != "" {
		return tag, nil
	}

	if pcx.GitHub != nil && (meta.Site == "" || meta.Site == "github.com") {
		tag, err := pcx.latestTagFromGitHubRelease(ctx, meta)
		if err == nil && tag != "" {
			return tag, nil
		}
		if err != nil {
			print.Verb(meta, "failed to get latest release tag:", err)
		}
	}

	_, _ = pcx.EnsureDependencyCached(meta, forceUpdate)
	return pcx.latestTagFromCache(meta)
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
	cachePath := meta.CachePath(pcx.CacheDir)
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
