package resource

import (
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func identifierFromDependencyMeta(meta versioning.DependencyMeta) string {
	site := meta.Site
	if site == "" {
		site = "github.com"
	}
	if meta.Scheme != "" {
		return filepath.Join(meta.Scheme, site, meta.User, meta.Repo)
	}
	return filepath.Join(site, meta.User, meta.Repo)
}
