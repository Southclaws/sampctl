package pawnpackage

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// RemotePackageFetcher loads package definitions from remote sources.
type RemotePackageFetcher interface {
	Fetch(ctx context.Context, meta versioning.DependencyMeta) (Package, error)
}
