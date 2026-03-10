package pkgcontext

import (
	git "github.com/go-git/go-git/v5"

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
		resolution.Resolved = meta.Commit[:8]
	default:
		resolution.Resolved = "HEAD"
	}

	return resolution, nil
}
