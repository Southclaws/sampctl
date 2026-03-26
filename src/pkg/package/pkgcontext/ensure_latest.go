package pkgcontext

import (
	"context"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func (pcx *PackageContext) resolveDynamicDependencyReference(
	ctx context.Context,
	effectiveMeta versioning.DependencyMeta,
	originalMeta versioning.DependencyMeta,
	forceUpdate bool,
) (versioning.DependencyMeta, error) {
	if effectiveMeta.Tag != "latest" {
		return effectiveMeta, nil
	}

	resolvedTag, err := pcx.resolveLatestTag(ctx, effectiveMeta, forceUpdate)
	if err != nil {
		return versioning.DependencyMeta{}, err
	}
	if resolvedTag == "" {
		return versioning.DependencyMeta{}, errors.New("latest did not resolve to a concrete tag")
	}

	updatedMeta := effectiveMeta
	updatedMeta.Tag = resolvedTag

	if previous, ok := pcx.PackageLockfileState.PreviousDependency(originalMeta); ok && previous.Resolved != "" && previous.Resolved != resolvedTag {
		print.Verb(originalMeta, "resolved latest from", previous.Resolved, "to", resolvedTag)
	}

	return updatedMeta, nil
}
