package pkgcontext

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func (pcx *PackageContext) ensureDependencyRepository(meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) (*git.Repository, error) {
	repo, err := git.PlainOpen(dependencyPath)
	if err == nil {
		head, headErr := repo.Head()
		if headErr != nil {
			print.Verb(meta, "existing repository has invalid HEAD, re-cloning")
			return pcx.recloneDependency(meta, dependencyPath)
		}
		print.Verb(meta, "repository already exists at", head.Hash().String()[:8])
		return repo, nil
	}

	if err != git.ErrRepositoryNotExists {
		print.Verb(meta, "error opening repository:", err)
		return pcx.recloneDependency(meta, dependencyPath)
	}

	print.Verb(meta, "repository does not exist, cloning from cache")
	return pcx.cloneDependencyFromCache(meta, dependencyPath)
}

func (pcx *PackageContext) cloneDependencyFromCache(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	repo, err := pcx.EnsureDependencyFromCache(meta, dependencyPath, false)
	if err != nil {
		print.Verb(meta, "failed to clone from cache:", err)
		os.RemoveAll(dependencyPath)
		return nil, errors.Wrap(err, "failed to clone dependency from cache")
	}

	valid, validationErr := ValidateRepository(dependencyPath)
	if validationErr != nil || !valid {
		print.Verb(meta, "cloned repository failed validation")
		os.RemoveAll(dependencyPath)
		if validationErr != nil {
			return nil, errors.Wrap(validationErr, "cloned repository is invalid")
		}
		return nil, errors.New("cloned repository failed validation")
	}

	return repo, nil
}

func (pcx *PackageContext) recloneDependency(meta versioning.DependencyMeta, dependencyPath string) (*git.Repository, error) {
	print.Verb(meta, "re-cloning dependency at", dependencyPath)

	if err := os.RemoveAll(dependencyPath); err != nil {
		return nil, errors.Wrap(err, "failed to remove corrupted dependency")
	}

	return pcx.cloneDependencyFromCache(meta, dependencyPath)
}

func (pcx *PackageContext) updateRepoStateWithRecovery(repo *git.Repository, meta versioning.DependencyMeta, dependencyPath string, forceUpdate bool) error {
	err := pcx.updateRepoState(repo, meta, forceUpdate)
	if err == nil {
		return nil
	}

	print.Verb(meta, "first update attempt failed:", err)

	if repairErr := RepairRepository(dependencyPath); repairErr == nil {
		print.Verb(meta, "repository repaired, retrying update")
		if repo, openErr := git.PlainOpen(dependencyPath); openErr == nil {
			if err = pcx.updateRepoState(repo, meta, true); err == nil {
				return nil
			}
		}
	}

	print.Verb(meta, "attempting force update")
	err = pcx.updateRepoState(repo, meta, true)
	if err == nil {
		return nil
	}

	print.Verb(meta, "all update attempts failed, re-cloning dependency")
	if _, cloneErr := pcx.recloneDependency(meta, dependencyPath); cloneErr != nil {
		return errors.Wrap(cloneErr, "failed to recover by re-cloning")
	}

	repo, err = git.PlainOpen(dependencyPath)
	if err != nil {
		return errors.Wrap(err, "failed to open re-cloned repository")
	}

	return pcx.updateRepoState(repo, meta, forceUpdate)
}

func (pcx *PackageContext) updateRepoState(
	repo *git.Repository,
	meta versioning.DependencyMeta,
	forcePull bool,
) error {
	print.Verb(meta, "updating repository state with", pcx.GitAuth, "authentication method")

	var (
		wt  *git.Worktree
		err error
	)

	if forcePull {
		print.Verb(meta, "performing forced pull to latest tip")
		repo, err = pcx.EnsureDependencyFromCache(meta, filepath.Join(pcx.Package.Vendor, meta.Repo), true)
		if err != nil {
			return errors.Wrap(err, "failed to ensure dependency in cache")
		}
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}

		if err = wt.Pull(&git.PullOptions{Depth: 1000}); err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to force pull for full update")
		}
	} else {
		wt, err = repo.Worktree()
		if err != nil {
			return errors.Wrap(err, "failed to get repo worktree")
		}
	}

	pullOpts := &git.PullOptions{}
	if meta.SSH != "" {
		pullOpts.Auth = pcx.GitAuth
	}

	var ref *plumbing.Reference
	switch {
	case meta.Tag != "":
		print.Verb(meta, "package has tag constraint:", meta.Tag)
		ref, err = versioning.RefFromTag(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from tag")
		}
	case meta.Branch != "":
		print.Verb(meta, "package has branch constraint:", meta.Branch)
		pullOpts.Depth = 1000
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + meta.Branch)
		if err = wt.Pull(pullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo branch")
		}
		ref, err = versioning.RefFromBranch(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from branch")
		}
	case meta.Commit != "":
		pullOpts.Depth = 1000
		if err = wt.Pull(pullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
			return errors.Wrap(err, "failed to pull repo")
		}
		ref, err = versioning.RefFromCommit(repo, meta)
		if err != nil {
			return errors.Wrap(err, "failed to get ref from commit")
		}
	}

	if ref != nil {
		if err = wt.Checkout(&git.CheckoutOptions{Hash: ref.Hash(), Force: true}); err != nil {
			return errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
		}
		print.Verb(meta, "successfully checked out to", ref.Hash())
		return nil
	}

	print.Verb(meta, "package does not have version constraint pulling latest")
	if err = wt.Pull(pullOpts); err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return errors.Wrap(err, "failed to fetch latest package")
	}

	return nil
}
