package download

import (
	"fmt"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// ReleaseAssetNotFoundError identifies a release without the requested asset.
type ReleaseAssetNotFoundError struct {
	Meta            versioning.DependencyMeta
	Tag             string
	Pattern         string
	AvailableAssets []string
}

// Error describes the selected release and its available assets.
func (e *ReleaseAssetNotFoundError) Error() string {
	return fmt.Sprintf("release %s/%s:%s has no asset matching %q; available assets: %v",
		e.Meta.User, e.Meta.Repo, e.Tag, e.Pattern, e.AvailableAssets)
}
