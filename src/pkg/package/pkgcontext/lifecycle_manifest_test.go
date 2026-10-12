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

func TestDependencyOperationPreservesManifestChangesSinceLoading(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			pkg := pawnpackage.Package{Format: format, LocalPath: dir, User: "fixture", Repo: "root", Entry: "original.pwn"}
			require.NoError(t, pkg.WriteDefinition())
			pcx, err := NewPackageContext(NewPackageContextOptions{Dir: dir, Parent: true, Init: true, CacheDir: t.TempDir(), Platform: "linux"})
			require.NoError(t, err)
			pkg.Entry = "edited.pwn"
			require.NoError(t, pkg.WriteDefinition())
			path := filepath.Join(dir, "pawn."+format)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			err = pcx.Install(context.Background(), nil, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "changed since loading")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.Equal(t, "original.pwn", pcx.Package.Entry)
			assert.NoFileExists(t, filepath.Join(dir, "pawn.lock"))
		})
	}
}

func TestDependencyOperationRefreshesLoadedManifestAfterSuccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pkg := pawnpackage.Package{Format: "json", LocalPath: dir, User: "fixture", Repo: "root"}
	require.NoError(t, pkg.WriteDefinition())
	require.NoError(t, os.Mkdir(filepath.Join(dir, "includes"), 0o700))
	pcx, err := NewPackageContext(NewPackageContextOptions{Dir: dir, Parent: true, Init: true, CacheDir: t.TempDir(), Platform: "linux", RuntimeProv: &fakeRuntimeProvisioner{}})
	require.NoError(t, err)
	require.NoError(t, pcx.Install(context.Background(), []versioning.DependencyString{"includes://local/includes"}, false))
	require.NoError(t, pcx.Install(context.Background(), nil, false))
	require.NoError(t, pcx.Uninstall([]versioning.DependencyString{"includes://local/includes"}, false))
	assert.Empty(t, pcx.Package.Dependencies)
	require.NoError(t, pcx.Install(context.Background(), nil, false))
}
