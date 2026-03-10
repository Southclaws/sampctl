package lockfile

import "github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"

// DependencyResolution describes the resolved state to persist for a dependency.
type DependencyResolution struct {
	Commit   string
	Resolved string
}

func defaultResolvedVersion(meta versioning.DependencyMeta, commitSHA string) string {
	switch {
	case meta.Tag != "":
		return meta.Tag
	case meta.Branch != "":
		return meta.Branch
	case meta.Commit != "":
		return meta.Commit[:8]
	case commitSHA != "":
		return "HEAD"
	default:
		return ""
	}
}
