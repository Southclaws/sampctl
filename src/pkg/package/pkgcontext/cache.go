package pkgcontext

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type repoEnsureRequest struct {
	Context     context.Context
	From        string
	To          string
	Branch      string
	SSH         bool
	FullClone   bool
	ForceUpdate bool
	CacheMeta   *versioning.DependencyMeta
}

type repoEnsureWithMetaRequest struct {
	Meta versioning.DependencyMeta
	repoEnsureRequest
}

// EnsureDependencyFromCache ensures the repository at `path` is up to date
func (pcx PackageContext) EnsureDependencyFromCache(
	meta versioning.DependencyMeta,
	path string,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	return pcx.EnsureDependencyFromCacheContext(context.Background(), meta, path, forceUpdate)
}

// EnsureDependencyFromCacheContext installs a cached repository while respecting cancellation.
func (pcx PackageContext) EnsureDependencyFromCacheContext(ctx context.Context, meta versioning.DependencyMeta, path string, forceUpdate bool) (repo *git.Repository, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	print.Verb(meta, "ensuring dependency package from cache to", path, "force update:", forceUpdate)

	from, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		err = errors.Wrap(err, "failed to make canonical path to cached copy")
		return
	}

	cacheRepoExists := fs.Exists(filepath.Join(from, ".git"))
	if !cacheRepoExists || forceUpdate {
		if forceUpdate && cacheRepoExists {
			noOrigin, originErr := pcx.cachedRepoHasNoOriginContext(ctx, meta)
			if originErr != nil {
				return nil, originErr
			}
			if noOrigin {
				print.Verb(meta, "cached repository has no origin remote, skipping cache refresh")
			} else {
				_, err = pcx.EnsureDependencyCachedContext(ctx, meta, forceUpdate)
				if err != nil {
					return
				}
			}
		} else {
			_, err = pcx.EnsureDependencyCachedContext(ctx, meta, forceUpdate)
			if err != nil {
				return
			}
		}
	}

	unlock, err := lockRepositoryPath(ctx, from)
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()

	if err := validateRepositoryDestination(from); err != nil {
		return nil, err
	}
	cacheRepo, err := pcx.repositoryStore().Open(from)
	if err != nil {
		return nil, err
	}
	if err := validateCachedRepositoryOrigin(cacheRepo, meta); err != nil {
		return nil, err
	}

	if destination, absErr := filepath.Abs(path); absErr != nil {
		return nil, absErr
	} else if destination == from {
		return pcx.PackageServices.repositoryStore().Open(from)
	}

	repo, err = pcx.ensureRepoExistsWithMeta(repoEnsureWithMetaRequest{
		Meta: meta,
		repoEnsureRequest: repoEnsureRequest{
			Context:     ctx,
			From:        from,
			To:          path,
			Branch:      meta.Branch,
			SSH:         meta.SSH != "",
			FullClone:   meta.Tag != "" || meta.Commit != "",
			ForceUpdate: forceUpdate,
		},
	})

	return
}

// EnsureDependencyCached clones a package to path using the default branch
func (pcx PackageContext) EnsureDependencyCached(
	meta versioning.DependencyMeta,
	forceUpdate bool,
) (repo *git.Repository, err error) {
	return pcx.EnsureDependencyCachedContext(context.Background(), meta, forceUpdate)
}

// EnsureDependencyCachedContext retrieves a cached repository while respecting cancellation.
func (pcx PackageContext) EnsureDependencyCachedContext(ctx context.Context, meta versioning.DependencyMeta, forceUpdate bool) (*git.Repository, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	to, err := pcx.cacheRepositoryPath(meta)
	if err != nil {
		return nil, err
	}

	if err := pcx.migrateLegacyRepositoryCache(ctx, meta, to); err != nil {
		return nil, err
	}

	return pcx.ensureRepoExistsWithMeta(repoEnsureWithMetaRequest{
		Meta: meta,
		repoEnsureRequest: repoEnsureRequest{
			Context:     ctx,
			From:        meta.URL(),
			To:          to,
			Branch:      meta.Branch,
			SSH:         meta.SSH != "",
			FullClone:   meta.Tag != "" || meta.Commit != "",
			ForceUpdate: forceUpdate,
			CacheMeta:   &meta,
		},
	})
}

func (pcx PackageContext) ensureRepoExists(request repoEnsureRequest) (repo *git.Repository, err error) {
	if request.Context == nil {
		request.Context = context.Background()
	}

	if err := request.Context.Err(); err != nil {
		return nil, err
	}

	if err := validateRepositoryDestination(request.To); err != nil {
		return nil, err
	}

	unlock, err := lockRepositoryPath(request.Context, request.To)
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, unlock()) }()

	if err := validateRepositoryDestination(request.To); err != nil {
		return nil, err
	}

	if fs.Exists(request.To) {
		valid, validationErr := pcx.PackageServices.repositoryHealth().Validate(request.To)
		if validationErr != nil {
			return nil, errors.Wrap(validationErr, "failed to validate repository")
		}
		if !valid {
			return pcx.cloneRepositoryContext(request)
		}
	}

	repo, err = pcx.PackageServices.repositoryStore().Open(request.To)
	if err != nil {
		if !stderrors.Is(err, git.ErrRepositoryNotExists) && !stderrors.Is(err, os.ErrNotExist) {
			return nil, errors.Wrap(err, "failed to open repository")
		}
		return pcx.cloneRepositoryContext(request)
	}

	if request.CacheMeta != nil {
		if err := validateCachedRepositoryOrigin(repo, *request.CacheMeta); err != nil {
			return nil, err
		}
	}

	if request.ForceUpdate {
		return pcx.updateRepositoryStaged(request)
	}

	return repo, nil
}

func (pcx PackageContext) cloneRepository(from, to, branch string, ssh bool, fullClone bool) (*git.Repository, error) {
	return pcx.cloneRepositoryContext(repoEnsureRequest{Context: context.Background(), From: from, To: to, Branch: branch, SSH: ssh, FullClone: fullClone})
}

func (pcx PackageContext) updateRepository(repo *git.Repository, request repoEnsureRequest) (*git.Repository, error) {
	if request.Context == nil {
		request.Context = context.Background()
	}

	wt, err := repo.Worktree()
	if err != nil {
		return pcx.cloneRepositoryContext(request)
	}

	pullOpts := &git.PullOptions{RemoteURL: request.From, Auth: pcx.authForRemote(request.From, request.SSH)}
	if request.Branch != "" {
		pullOpts.ReferenceName = plumbing.ReferenceName("refs/heads/" + request.Branch)
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = wt.PullContext(request.Context, pullOpts)
		if err == nil || stderrors.Is(err, git.NoErrAlreadyUpToDate) {
			return repo, nil
		}
		if !isTransientGitError(err) || attempt == 2 {
			break
		}
		if err := waitGitRetry(request.Context, attempt); err != nil {
			return nil, err
		}
	}

	if ctxErr := request.Context.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) || !isRepositoryCorrupt(request.To) {
		return nil, errors.Wrap(err, "failed to update repository")
	}

	return pcx.cloneRepositoryContext(request)
}

func (pcx PackageContext) recoverByReclone(from, to, branch string, ssh bool, fullClone bool) (*git.Repository, error) {
	return pcx.cloneRepository(from, to, branch, ssh, fullClone)
}

// ensureRepoExistsWithMeta wraps ensureRepoExists and provides improved error handling
func (pcx PackageContext) ensureRepoExistsWithMeta(request repoEnsureWithMetaRequest) (repo *git.Repository, err error) {
	repo, err = pcx.ensureRepoExists(request.repoEnsureRequest)
	if err != nil && pcx.shouldRetryWithSSH(request.Meta, request.From, request.SSH, err) {
		fallbackFrom := toGitSSHURL(request.Meta)
		print.Verb(request.Meta, "HTTPS git operation failed, retrying with SSH:", fallbackFrom)
		repo, err = pcx.ensureRepoExists(repoEnsureRequest{
			Context:     request.Context,
			From:        fallbackFrom,
			To:          request.To,
			Branch:      request.Branch,
			SSH:         true,
			FullClone:   request.FullClone,
			ForceUpdate: request.ForceUpdate,
			CacheMeta:   request.CacheMeta,
		})
	}

	if err != nil {
		err = WrapGitError(err, request.Meta)
	}

	return repo, err
}
