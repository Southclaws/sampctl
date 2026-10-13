package pkgcontext

import (
	"context"
	stderrors "errors"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func (pcx *PackageContext) ensureDependencyRepository(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	return pcx.ensureDependencyRepositoryContext(context.Background(), meta, dependencyPath)
}

func (pcx *PackageContext) ensureDependencyRepositoryContext(ctx context.Context, meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repo, err := pcx.PackageServices.repositoryStore().Open(dependencyPath)
	if err == nil {
		head, headErr := repo.Head()
		if headErr != nil {
			print.Verb(meta, "existing repository has invalid HEAD, re-cloning")
			return pcx.recloneDependencyContext(ctx, meta, dependencyPath)
		}
		print.Verb(meta, "repository already exists at", head.Hash().String()[:8])
		return repo, nil
	}

	if err != git.ErrRepositoryNotExists {
		print.Verb(meta, "unable to open dependency repository")
		return pcx.recloneDependencyContext(ctx, meta, dependencyPath)
	}

	print.Verb(meta, "repository does not exist, cloning from cache")
	return pcx.cloneDependencyFromCacheContext(ctx, meta, dependencyPath)
}

func (pcx *PackageContext) cloneDependencyFromCache(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	return pcx.cloneDependencyFromCacheContext(context.Background(), meta, dependencyPath)
}

func (pcx *PackageContext) cloneDependencyFromCacheContext(ctx context.Context, meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repo, err := pcx.EnsureDependencyFromCacheContext(ctx, meta, dependencyPath, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to clone dependency from cache")
	}

	valid, validationErr := pcx.PackageServices.repositoryHealth().Validate(dependencyPath)
	if validationErr != nil || !valid {
		print.Verb(meta, "cloned repository failed validation")
		if validationErr == nil {
			validationErr = errors.New("cloned repository failed validation")
		}
		return nil, errors.Wrap(validationErr, "cloned repository is invalid")
	}

	return repo, nil
}

func (pcx *PackageContext) recloneDependency(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	return pcx.recloneDependencyContext(context.Background(), meta, dependencyPath)
}

func (pcx *PackageContext) recloneDependencyContext(ctx context.Context, meta versioning.DependencyMeta, dependencyPath string) (_ *git.Repository, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	print.Verb(meta, "re-cloning dependency at", dependencyPath)

	if _, err := pcx.EnsureDependencyCachedContext(ctx, meta, false); err != nil {
		return nil, errors.Wrap(err, "failed to ensure cached dependency for recovery")
	}
	from, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		return nil, err
	}
	unlock, err := lockRepositoryPath(ctx, from)
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()
	if err := validateRepositoryDestination(from); err != nil {
		return nil, err
	}
	if err := validateRepositoryDestination(dependencyPath); err != nil {
		return nil, err
	}
	return pcx.cloneRepositoryContext(repoEnsureRequest{Context: ctx, From: from, To: dependencyPath, Branch: meta.Branch, FullClone: true})
}

func (pcx *PackageContext) updateRepoStateWithRecovery(repo *git.Repository, meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) error {
	return pcx.updateRepoStateWithRecoveryContext(context.Background(), repo, meta, dependencyPath, forceUpdate)
}

func (pcx *PackageContext) updateRepoStateWithRecoveryContext(ctx context.Context, repo *git.Repository, meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) error {
	err := pcx.updateRepoStateContext(ctx, repo, meta, forceUpdate)
	if err == nil {
		return nil
	}

	if ctx.Err() != nil || stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return err
	}
	valid, validationErr := pcx.PackageServices.repositoryHealth().Validate(dependencyPath)
	if validationErr != nil || valid {
		return err
	}
	print.Verb(meta, "repository update failed and repository is invalid")
	if repairErr := pcx.PackageServices.repositoryHealth().Repair(dependencyPath); repairErr == nil {
		print.Verb(meta, "repository repaired, retrying update")
		if repo, openErr := pcx.PackageServices.repositoryStore().Open(dependencyPath); openErr == nil {
			if retryErr := pcx.updateRepoStateContext(ctx, repo, meta, forceUpdate); retryErr == nil {
				return nil
			} else if ctx.Err() != nil || stderrors.Is(retryErr, context.Canceled) || stderrors.Is(retryErr, context.DeadlineExceeded) {
				return retryErr
			}
		}
	}

	if _, cloneErr := pcx.recloneDependencyContext(ctx, meta, dependencyPath); cloneErr != nil {
		return stderrors.Join(err, errors.Wrap(cloneErr, "failed to recover by re-cloning"))
	}

	repo, err = pcx.PackageServices.repositoryStore().Open(dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to open re-cloned repository")
	}

	return pcx.updateRepoStateContext(ctx, repo, meta, forceUpdate)
}

func (pcx *PackageContext) updateRepoState(
	repo *git.Repository,
	meta versioning.DependencyMeta,
	forcePull bool,
) error {
	return pcx.updateRepoStateContext(context.Background(), repo, meta, forcePull)
}

func (pcx *PackageContext) updateRepoStateContext(ctx context.Context, repo *git.Repository, meta versioning.DependencyMeta, forcePull bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	print.Verb(meta, "updating repository state")
	remoteURL := getRepositoryOriginURL(repo)

	var (
		wt  *git.Worktree
		err error
	)

	if forcePull {
		print.Verb(meta, "performing forced pull to latest tip")
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}
		repo, err = pcx.EnsureDependencyFromCacheContext(ctx, meta, wt.Filesystem.Root(), true)
		if err != nil {
			return errors.Wrap(err, "failed to ensure dependency in cache")
		}
		remoteURL = getRepositoryOriginURL(repo)
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}
		forcePullOpts := &git.PullOptions{Depth: 1000}
		if auth := pcx.authForRemote(remoteURL, meta.SSH != ""); auth != nil {
			forcePullOpts.Auth = auth
		}

		if err = wt.PullContext(ctx, forcePullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to force pull for full update")
		}
	} else {
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}
	}

	pullOpts := &git.PullOptions{}
	if auth := pcx.authForRemote(remoteURL, meta.SSH != ""); auth != nil {
		pullOpts.Auth = auth
	}

	var ref *plumbing.Reference
	switch {
	case meta.Tag != "":
		print.Verb(meta, "package has tag constraint:", meta.Tag)
		ref, err = versioning.RefFromTag(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from tag")
		}
	case meta.Branch != "" && meta.Commit == "":
		print.Verb(meta, "package has branch constraint:", meta.Branch)
		pullOpts.Depth = 1000
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + meta.Branch)
		ref, err = versioning.RefFromBranch(repo, meta)
		if err != nil {
			if err = wt.PullContext(ctx, pullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
				return errors.Wrap(err, "failed to pull repo branch")
			}
			ref, err = versioning.RefFromBranch(repo, meta)
		}
		if err != nil {
			return errors.Wrap(err, "failed to get ref from branch")
		}
	case meta.Commit != "":
		pullOpts.Depth = 1000
		ref, err = versioning.RefFromCommit(repo, meta)
		if err != nil {
			if err = wt.PullContext(ctx, pullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
				return errors.Wrap(err, "failed to pull repo")
			}
			ref, err = versioning.RefFromCommit(repo, meta)
		}
		if err != nil {
			return errors.Wrap(err, "failed to get ref from commit")
		}
	}

	if ref != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err = wt.Checkout(&git.CheckoutOptions{Hash: ref.Hash(), Force: true}); err != nil {
			return errors.Wrapf(err, "failed to check out requested commit %s", ref.Hash())
		}
		print.Verb(meta, "checked out commit", ref.Hash())
		return nil
	}

	print.Verb(meta, "has no version constraint. Pulling the latest commit")
	if err = wt.PullContext(ctx, pullOpts); err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return errors.Wrap(err, "failed to fetch latest package")
	}

	return nil
}

func getRepositoryOriginURL(repo *git.Repository) string {
	remote, err := repo.Remote("origin")
	if err != nil {
		return ""
	}
	if remote == nil || remote.Config() == nil || len(remote.Config().URLs) == 0 {
		return ""
	}
	return remote.Config().URLs[0]
}
