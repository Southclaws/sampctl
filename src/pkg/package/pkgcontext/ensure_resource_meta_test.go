package pkgcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestApplyDependencyMetaToPackage(t *testing.T) {
	t.Parallel()

	pkg := pawnpackage.Package{}
	meta := versioning.DependencyMeta{
		Site:   "github.com",
		User:   "ADRFranklin",
		Repo:   "community-anticheat",
		Tag:    "1.0.0",
		Branch: "release",
		Commit: "0123456789012345678901234567890123456789",
		SSH:    "git",
	}

	applyDependencyMetaToPackage(&pkg, meta)

	assert.Equal(t, meta.Site, pkg.Site)
	assert.Equal(t, meta.User, pkg.User)
	assert.Equal(t, meta.Repo, pkg.Repo)
	assert.Equal(t, meta.Tag, pkg.Tag)
	assert.Equal(t, meta.Branch, pkg.Branch)
	assert.Equal(t, meta.Commit, pkg.Commit)
	assert.Equal(t, meta.SSH, pkg.SSH)
}
