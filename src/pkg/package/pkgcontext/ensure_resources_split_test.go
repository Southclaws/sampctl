package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestResourcePackageDefinitionFallsBackToLocalPackage(t *testing.T) {
	t.Parallel()

	vendorDir := t.TempDir()
	depDir := filepath.Join(vendorDir, "dep")
	require.NoError(t, os.MkdirAll(depDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(depDir, "pawn.json"), []byte(`{"runtime":{"version":"local-version"}}`), 0o644))

	fetcher := &fakeRemotePackageFetcher{pkg: pawnpackage.Package{Format: "json"}}
	pcx := &PackageContext{
		Package: pawnpackage.Package{Vendor: vendorDir},
		PackageServices: PackageServices{
			CacheDir:       t.TempDir(),
			RemotePackages: fetcher,
		},
	}

	pkg, err := pcx.resourcePackageDefinition(context.Background(), versioning.DependencyMeta{Repo: "dep"})
	require.NoError(t, err)
	assert.Equal(t, "json", pkg.Format)
	require.NotNil(t, pkg.Runtime)
	assert.Equal(t, "local-version", pkg.Runtime.Version)
	assert.False(t, fetcher.called)
}

func TestResourcePackageDefinitionFallsBackToRemotePackage(t *testing.T) {
	t.Parallel()

	fetcher := &fakeRemotePackageFetcher{pkg: pawnpackage.Package{Format: "yaml", Repo: "dep"}}
	pcx := &PackageContext{
		Package: pawnpackage.Package{Vendor: t.TempDir()},
		PackageServices: PackageServices{
			CacheDir:       t.TempDir(),
			RemotePackages: fetcher,
		},
	}

	pkg, err := pcx.resourcePackageDefinition(context.Background(), versioning.DependencyMeta{Repo: "dep"})
	require.NoError(t, err)
	assert.True(t, fetcher.called)
	assert.Equal(t, "yaml", pkg.Format)
	assert.Equal(t, "dep", pkg.Repo)
}

func TestResourcePackageDefinitionReturnsEmptyPackageWithoutRemoteFetcher(t *testing.T) {
	t.Parallel()

	pcx := &PackageContext{
		Package:         pawnpackage.Package{Vendor: t.TempDir()},
		PackageServices: PackageServices{CacheDir: t.TempDir()},
	}

	pkg, err := pcx.resourcePackageDefinition(context.Background(), versioning.DependencyMeta{Repo: "missing"})
	require.NoError(t, err)
	assert.Empty(t, pkg.Format)
	assert.Nil(t, pkg.Runtime)
}
