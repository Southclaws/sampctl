package pkgcontext

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type fakeRemotePackageFetcher struct {
	called bool
	pkg    pawnpackage.Package
	err    error
}

func (f *fakeRemotePackageFetcher) Fetch(_ context.Context, _ versioning.DependencyMeta) (pawnpackage.Package, error) {
	f.called = true
	return f.pkg, f.err
}

func TestInstallPackageResourcesUsesInjectedRemoteFetcher(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	vendorDir := t.TempDir()
	fetcher := &fakeRemotePackageFetcher{pkg: pawnpackage.Package{}}

	pcx := &PackageContext{
		CacheDir:       t.TempDir(),
		Platform:       "linux",
		RemotePackages: fetcher,
		Package: pawnpackage.Package{
			LocalPath: projectDir,
			Vendor:    vendorDir,
		},
	}

	err := pcx.installPackageResources(versioning.DependencyMeta{User: "fixture", Repo: "repo"})
	require.NoError(t, err)
	assert.True(t, fetcher.called)
}
