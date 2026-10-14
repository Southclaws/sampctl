package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func cachedRepositoryOrigin(repo *git.Repository) (string, error) {
	remote, err := repo.Remote("origin")
	if errors.Is(err, git.ErrRemoteNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if remote == nil || remote.Config() == nil || len(remote.Config().URLs) == 0 {
		return "", nil
	}

	return remote.Config().URLs[0], nil
}

func repositoryOriginMatches(origin string, meta versioning.DependencyMeta) bool {
	endpoint, err := transport.NewEndpoint(origin)
	if err != nil || endpoint.Host == "" {
		return false
	}

	meta.Site = normalizeDependencySite(meta.Site)
	expectedEndpoint, err := transport.NewEndpoint(meta.URL())
	if err != nil || !strings.EqualFold(repositoryEndpointHost(endpoint), repositoryEndpointHost(expectedEndpoint)) {
		return false
	}

	path := strings.TrimSuffix(strings.Trim(endpoint.Path, "/"), ".git")
	expected := meta.User + "/" + strings.TrimSuffix(meta.Repo, ".git")
	if strings.EqualFold(expectedEndpoint.Host, "github.com") {
		return strings.EqualFold(path, expected)
	}

	return path == expected
}

func repositoryEndpointHost(endpoint *transport.Endpoint) string {
	defaultPort := 0
	switch endpoint.Protocol {
	case "https":
		defaultPort = 443
	case "http":
		defaultPort = 80
	case "ssh":
		defaultPort = 22
	case "git":
		defaultPort = 9418
	}
	if endpoint.Port == 0 || endpoint.Port == defaultPort {
		return endpoint.Host
	}

	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}

func validateCachedRepositoryOrigin(repo *git.Repository, meta versioning.DependencyMeta) error {
	origin, err := cachedRepositoryOrigin(repo)
	if err != nil {
		return fmt.Errorf("read cached repository origin: %w", err)
	}
	// Origin-less repositories support explicitly seeded offline caches. Legacy
	// cache migration requires positive identity evidence instead.
	if origin == "" || repositoryOriginMatches(origin, meta) {
		return nil
	}

	return fmt.Errorf("cached repository origin does not match dependency %s: %w", meta, os.ErrInvalid)
}

func (pcx PackageContext) migrateLegacyRepositoryCache(ctx context.Context, meta versioning.DependencyMeta, destination string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	branch := meta.Branch
	if branch == "" {
		branch = "default"
	}

	root, err := filepath.Abs(pcx.CacheDir)
	if err != nil {
		return err
	}

	legacy := filepath.Join(root, "packages", meta.User, meta.Repo, branch)
	if _, err := os.Lstat(filepath.Join(legacy, ".git")); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := validateCacheRepositoryPath(root, legacy); err != nil {
		return err
	}

	unlockLegacy, err := lockRepositoryPath(ctx, legacy)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlockLegacy()) }()

	unlockDestination, err := lockRepositoryPath(ctx, destination)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlockDestination()) }()

	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := validateRepositoryDestination(legacy); err != nil {
		return err
	}

	repo, err := pcx.repositoryStore().Open(legacy)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return nil
	}
	if err != nil {
		return err
	}
	origin, err := cachedRepositoryOrigin(repo)
	if err != nil {
		return err
	}
	if !repositoryOriginMatches(origin, meta) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(legacy, destination); err != nil {
		return fmt.Errorf("migrate repository cache: %w", err)
	}

	return errors.Join(fs.SyncDir(filepath.Dir(legacy)), fs.SyncDir(filepath.Dir(destination)))
}
