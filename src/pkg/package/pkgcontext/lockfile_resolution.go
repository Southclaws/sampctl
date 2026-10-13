package pkgcontext

import (
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func resolveDependencyLock(meta versioning.DependencyMeta, repo *git.Repository) (lockfile.DependencyResolution, error) {
	head, err := repo.Head()
	if err != nil {
		return lockfile.DependencyResolution{}, err
	}

	resolution := lockfile.DependencyResolution{Commit: head.Hash().String()}

	tag, err := versioning.GetRepoCurrentVersionedTag(repo)
	if err == nil && tag != nil {
		resolution.Resolved = tag.Name
		return resolution, nil
	}

	switch {
	case meta.Tag != "":
		resolution.Resolved = meta.Tag
	case meta.Branch != "":
		resolution.Resolved = meta.Branch
	case meta.Commit != "":
		resolution.Resolved = meta.Commit[:min(8, len(meta.Commit))]
	default:
		resolution.Resolved = "HEAD"
	}

	return resolution, nil
}

func resolveCachedDependencyLock(meta versioning.DependencyMeta, repo *git.Repository) (lockfile.DependencyResolution, error) {
	if meta.Tag == "" && meta.Commit == "" {
		return resolveDependencyLock(meta, repo)
	}
	var ref *plumbing.Reference
	var err error
	resolved := meta.Tag
	if meta.Commit != "" {
		ref, err = versioning.RefFromCommit(repo, meta)
		resolved = meta.Commit[:min(8, len(meta.Commit))]
	} else {
		ref, err = versioning.RefFromTag(repo, meta)
	}
	if err != nil {
		return lockfile.DependencyResolution{}, err
	}
	return lockfile.DependencyResolution{Commit: ref.Hash().String(), Resolved: resolved}, nil
}
