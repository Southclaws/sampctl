package pkgcontext

import (
	"context"
	"errors"

	"github.com/go-git/go-git/v5"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

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

	repo, err := pcx.repositoryStore().Open(path)
	if errors.Is(err, git.ErrRepositoryNotExists) && (meta.Tag == "" || meta.Tag == "latest") && meta.Commit == "" {
		return pawnpackage.PackageFromDir(path)
	}
	if err != nil {
		return pkg, err
	}
	if err := validateCachedRepositoryOrigin(repo, meta); err != nil {
		return pkg, err
	}

	return pcx.readPackageFromCachedRevision(meta, path)
}
