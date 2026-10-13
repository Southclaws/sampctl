package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/gofrs/flock"
	"github.com/otiai10/copy"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

// CloneContext clones a repository with cancellation support.
func (GitRepositoryStore) CloneContext(ctx context.Context, path string, isBare bool, opts *git.CloneOptions) (*git.Repository, error) {
	return git.PlainCloneContext(ctx, path, isBare, opts)
}

func (pcx PackageContext) cacheRepositoryPath(meta versioning.DependencyMeta) (string, error) {
	if err := meta.Validate(); err != nil {
		return "", fmt.Errorf("invalid dependency %s: %w", meta, err)
	}
	for _, component := range []string{meta.User, meta.Repo} {
		if component == "." || component == ".." || strings.ContainsAny(component, `/\`) {
			return "", fmt.Errorf("invalid repository path for dependency %s", meta)
		}
	}
	for _, component := range strings.Split(strings.ReplaceAll(meta.Branch, `\`, "/"), "/") {
		if component == "." || component == ".." {
			return "", fmt.Errorf("invalid branch path for dependency %s", meta)
		}
	}
	root, err := filepath.Abs(pcx.CacheDir)
	if err != nil {
		return "", err
	}
	path := meta.CachePath(root)
	for current := path; current != root; current = filepath.Dir(current) {
		if current == filepath.Dir(current) {
			return "", fmt.Errorf("repository cache path escapes cache directory: %s", path)
		}
		info, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("repository cache path contains a symbolic link %s: %w", current, os.ErrInvalid)
		}
	}
	return path, nil
}

func validateRepositoryDestination(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository destination must be a directory without a symbolic link %s: %w", path, os.ErrInvalid)
	}
	gitInfo, err := os.Lstat(filepath.Join(path, ".git"))
	if err == nil {
		if !gitInfo.IsDir() || gitInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository metadata must be a directory without a symbolic link %s: %w", path, os.ErrInvalid)
		}
		return filepath.WalkDir(filepath.Join(path, ".git"), func(current string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("repository metadata contains a symbolic link %s: %w", current, os.ErrInvalid)
			}
			return nil
		})
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("refusing to replace a nonempty directory without repository metadata %s: %w", path, os.ErrInvalid)
	}
	return nil
}

func lockRepositoryPath(ctx context.Context, path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create repository parent directory: %w", err)
	}
	lockPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".sampctl.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("repository lock is not a regular file %s: %w", lockPath, os.ErrInvalid)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lock := flock.New(lockPath)
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire repository lock: %w", errors.Join(err, lock.Close()))
	}
	if !locked {
		return nil, errors.Join(ctx.Err(), lock.Close())
	}
	return lock.Close, nil
}

func (pcx PackageContext) packageFromCachedRevisionContext(ctx context.Context, meta versioning.DependencyMeta, path string) (pkg pawnpackage.Package, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	unlock, err := lockRepositoryPath(ctx, path)
	if err != nil {
		return pkg, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	if err := validateRepositoryDestination(path); err != nil {
		return pkg, err
	}
	return pcx.readPackageFromCachedRevision(meta, path)
}

func (pcx PackageContext) cloneRepositoryContext(request repoEnsureRequest) (_ *git.Repository, err error) {
	if request.Context == nil {
		request.Context = context.Background()
	}
	if err := request.Context.Err(); err != nil {
		return nil, err
	}
	if err := validateRepositoryDestination(request.To); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(request.To), 0o700); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(request.To), "."+filepath.Base(request.To)+".clone-")
	if err != nil {
		return nil, fmt.Errorf("failed to create repository staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
				err = errors.Join(err, &OperationError{Operation: "cleanup", Err: fmt.Errorf("failed to clean repository staging directory %s: %w", stage, cleanupErr)})
			}
		}
	}()
	stagedPath := filepath.Join(stage, "repository")
	if err := pcx.cloneStagedRepository(request, stagedPath); err != nil {
		return nil, err
	}
	if err := request.Context.Err(); err != nil {
		return nil, err
	}
	if err := validateRepositoryDestination(request.To); err != nil {
		return nil, err
	}
	repo, preserve, err := pcx.promoteRepository(stagedPath, request.To, filepath.Join(stage, "previous"))
	cleanup = !preserve
	return repo, err
}

func (pcx PackageContext) updateRepositoryStaged(request repoEnsureRequest) (_ *git.Repository, err error) {
	stage, err := os.MkdirTemp(filepath.Dir(request.To), "."+filepath.Base(request.To)+".update-")
	if err != nil {
		return nil, fmt.Errorf("failed to create repository update staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
				err = errors.Join(err, &OperationError{Operation: "cleanup", Err: fmt.Errorf("failed to clean repository update staging directory %s: %w", stage, cleanupErr)})
			}
		}
	}()
	stagedPath := filepath.Join(stage, "repository")
	if err := copy.Copy(request.To, stagedPath, copy.Options{
		OnSymlink:  func(string) copy.SymlinkAction { return copy.Shallow },
		Skip:       func(os.FileInfo, string, string) (bool, error) { return false, request.Context.Err() },
		WrapReader: func(reader io.Reader) io.Reader { return dependencyContextReader{ctx: request.Context, reader: reader} },
	}); err != nil {
		return nil, fmt.Errorf("failed to stage repository update: %w", err)
	}
	repo, err := pcx.PackageServices.repositoryStore().Open(stagedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open staged repository: %w", err)
	}
	stagedRequest := request
	stagedRequest.To = stagedPath
	if _, err := pcx.updateRepository(repo, stagedRequest); err != nil {
		return nil, err
	}
	if err := request.Context.Err(); err != nil {
		return nil, err
	}
	valid, err := pcx.PackageServices.repositoryHealth().Validate(stagedPath)
	if err != nil {
		return nil, fmt.Errorf("updated repository is invalid: %w", err)
	}
	if !valid {
		return nil, errors.New("updated repository failed validation")
	}
	repo, preserve, err := pcx.promoteRepository(stagedPath, request.To, filepath.Join(stage, "previous"))
	cleanup = !preserve
	return repo, err
}

func (pcx PackageContext) cloneStagedRepository(request repoEnsureRequest, path string) error {
	opts := &git.CloneOptions{URL: request.From, Depth: 1000, Auth: pcx.authForRemote(request.From, request.SSH)}
	if request.FullClone {
		opts.Depth = 0
	}
	if request.Branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(request.Branch)
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		store := pcx.PackageServices.repositoryStore()
		if contextual, ok := store.(interface {
			CloneContext(context.Context, string, bool, *git.CloneOptions) (*git.Repository, error)
		}); ok {
			_, err = contextual.CloneContext(request.Context, path, false, opts)
		} else {
			_, err = store.Clone(path, false, opts)
		}
		if ctxErr := request.Context.Err(); ctxErr != nil {
			return errors.Join(ctxErr, err)
		}
		if err == nil {
			break
		}
		if !isTransientGitError(err) || attempt == 2 {
			return fmt.Errorf("failed to clone repository: %w", err)
		}
		if cleanupErr := os.RemoveAll(path); cleanupErr != nil {
			return errors.Join(err, &OperationError{Operation: "cleanup", Err: fmt.Errorf("failed to clean interrupted clone: %w", cleanupErr)})
		}
		if err := waitGitRetry(request.Context, attempt); err != nil {
			return err
		}
	}
	valid, err := pcx.PackageServices.repositoryHealth().Validate(path)
	if err != nil {
		return fmt.Errorf("cloned repository is invalid: %w", err)
	}
	if !valid {
		return errors.New("cloned repository failed validation")
	}
	return nil
}

func (pcx PackageContext) promoteRepository(from, to, backup string) (*git.Repository, bool, error) {
	hadPrevious := false
	if _, err := os.Lstat(to); err == nil {
		if err := os.Rename(to, backup); err != nil {
			return nil, false, fmt.Errorf("failed to preserve previous repository: %w", err)
		}
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	if err := os.Rename(from, to); err != nil {
		if hadPrevious {
			if restoreErr := os.Rename(backup, to); restoreErr != nil {
				return nil, true, errors.Join(err, &OperationError{Operation: "rollback", Err: fmt.Errorf("failed to restore previous repository. Backup kept at %s: %w", backup, restoreErr)})
			}
		}
		return nil, false, fmt.Errorf("failed to promote cloned repository: %w", err)
	}
	repo, err := pcx.PackageServices.repositoryStore().Open(to)
	if err == nil {
		return repo, false, nil
	}
	if restoreErr := os.Rename(to, from); restoreErr != nil {
		return nil, true, errors.Join(err, &OperationError{Operation: "rollback", Err: fmt.Errorf("failed to withdraw invalid repository. Previous copy kept at %s: %w", backup, restoreErr)})
	}
	if hadPrevious {
		if restoreErr := os.Rename(backup, to); restoreErr != nil {
			return nil, true, errors.Join(err, &OperationError{Operation: "rollback", Err: fmt.Errorf("failed to restore previous repository. Backup kept at %s: %w", backup, restoreErr)})
		}
	}
	return nil, false, fmt.Errorf("failed to open promoted repository: %w", err)
}

func isRepositoryCorrupt(path string) bool {
	valid, err := ValidateRepository(path)
	return err == nil && !valid
}

func waitGitRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(1<<attempt) * 100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
