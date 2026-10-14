package lockfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
)

// OpenRepositoryContext opens a repository after checking files used by Git status.
func OpenRepositoryContext(ctx context.Context, dir string) (*git.Repository, error) {
	if err := ValidateWorktreeFilesContext(ctx, dir); err != nil {
		return nil, err
	}

	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return nil, err
	}

	if err := validateRegisteredSubmodulesContext(ctx, repo); err != nil {
		return nil, err
	}

	// Keep status read-only if module HEAD disappears after the preflight.
	repo.Storer = statusStorage{Storer: repo.Storer}
	return repo, ctx.Err()
}

type statusStorage struct {
	storage.Storer
}

func (s statusStorage) Module(name string) (storage.Storer, error) {
	module, err := s.Storer.Module(name)
	if err != nil {
		return nil, err
	}

	return statusStorage{Storer: module}, nil
}

func (statusStorage) SetReference(*plumbing.Reference) error {
	return billy.ErrReadOnly
}

func validateRegisteredSubmodulesContext(ctx context.Context, repo *git.Repository) error {
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	submodules, err := wt.Submodules()
	if err != nil {
		return err
	}

	cfg, err := repo.Config()
	if err != nil {
		return err
	}

	for _, submodule := range submodules {
		if err := ctx.Err(); err != nil {
			return err
		}

		name := submodule.Config().Name
		if cfg.Submodules[name] == nil {
			continue
		}

		module, err := repo.Storer.Module(name)
		if err != nil {
			return fmt.Errorf("inspect registered submodule %q: %w", name, err)
		}

		// Status initializes registered submodules without HEAD, writing new files.
		if _, err := module.Reference(plumbing.HEAD); err != nil {
			return fmt.Errorf("registered submodule %q has no readable HEAD: %w", name, err)
		}
	}

	return ctx.Err()
}

func validateGitMetadataFilesContext(ctx context.Context, dir string) error {
	path := filepath.Join(dir, ".git")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		path, err = readGitDirectoryFile(ctx, path, dir)
		if err != nil {
			return err
		}
	}

	return validateGitMetadataDirectory(ctx, path, make(map[string]bool))
}

func readGitDirectoryFile(ctx context.Context, path, worktree string) (string, error) {
	info, err := integrityRegularFile(path)
	if err != nil {
		return "", err
	}
	if info.Size() > 64*1024 {
		return "", fmt.Errorf("git directory pointer %q exceeds 64 KiB", path)
	}

	var contents strings.Builder
	if err := hashFileContentContext(ctx, &contents, path); err != nil {
		return "", err
	}

	const prefix = "gitdir: "
	line := contents.String()
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("git directory pointer %q has no gitdir prefix", path)
	}

	gitdir := strings.TrimSpace(strings.SplitN(line[len(prefix):], "\n", 2)[0])
	if gitdir == "" {
		return "", fmt.Errorf("git directory pointer %q is empty", path)
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktree, gitdir)
	}

	return gitdir, nil
}

func validateGitMetadataDirectory(ctx context.Context, path string, visited map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	root, err := resolveIntegrityDirectory(path)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}

	if visited[root] {
		return nil
	}
	visited[root] = true

	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		if walkErr != nil {
			return walkErr
		}

		return validateGitMetadataEntry(ctx, path, info, visited)
	})
	if err != nil {
		return err
	}

	if err := validateGitCommonDirectoryContext(ctx, root, visited); err != nil {
		return err
	}

	return validateGitAlternatesContext(ctx, root, visited)
}

func validateGitCommonDirectoryContext(ctx context.Context, root string, visited map[string]bool) error {
	path := filepath.Join(root, "commondir")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	info, err := integrityRegularFile(path)
	if err != nil {
		return err
	}
	if info.Size() > 64*1024 {
		return fmt.Errorf("git common directory pointer %q exceeds 64 KiB", path)
	}

	var contents strings.Builder
	if err := hashFileContentContext(ctx, &contents, path); err != nil {
		return err
	}

	common := strings.TrimSpace(contents.String())
	if common == "" {
		return fmt.Errorf("git common directory pointer %q is empty", path)
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(root, common)
	}

	return validateGitMetadataDirectory(ctx, common, visited)
}

func validateGitMetadataEntry(ctx context.Context, path string, info os.FileInfo, visited map[string]bool) error {
	if info.IsDir() || info.Mode().IsRegular() {
		return nil
	}

	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := os.Stat(path)
		if err != nil {
			return err
		}
		if resolved.IsDir() {
			return validateGitMetadataDirectory(ctx, path, visited)
		}
		if resolved.Mode().IsRegular() {
			return nil
		}
	}

	return fmt.Errorf("git metadata file %q is not a regular file or directory", path)
}

func validateGitAlternatesContext(ctx context.Context, root string, visited map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	alternates, err := dotgit.New(osfs.New(root)).Alternates()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, alternate := range alternates {
		if err := validateGitMetadataDirectory(ctx, alternate.Fs().Root(), visited); err != nil {
			return err
		}
	}

	return ctx.Err()
}
