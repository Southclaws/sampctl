package pkgcontext

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestDependencyUpdatePreservesHostTransportAndSubpath(t *testing.T) {
	t.Parallel()

	for _, declaration := range []versioning.DependencyString{"https://gitlab.com/fixture/shared/includes", "deploy@gitlab.com:fixture/shared/includes", "git@github.com:fixture/shared/includes", "includes://fixture/shared/includes"} {
		t.Run(string(declaration), func(t *testing.T) {
			t.Parallel()

			pcx := lifecycleContext(t, "json")
			pcx.Package.Dependencies = []versioning.DependencyString{declaration}
			require.NoError(t, pcx.Package.WriteDefinition())
			original, err := declaration.Explode()
			require.NoError(t, err)
			seedReliabilityRepo(t, original.CachePath(pcx.CacheDir), `{}`)

			updated, err := pcx.UpdateDependencyReferences(t.Context(), DependencyUpdateRequest{Enabled: true})
			require.NoError(t, err)
			require.True(t, updated)
			written, err := pawnpackage.PackageFromDir(pcx.Package.LocalPath)
			require.NoError(t, err)
			require.Len(t, written.Dependencies, 1)
			resolved, err := written.Dependencies[0].Explode()
			require.NoError(t, err)
			original.Tag = "latest"
			require.Equal(t, original, resolved)
		})
	}
}

func TestRemoteSchemesInstallAndLockTransitiveDependencies(t *testing.T) {
	t.Parallel()

	for _, scheme := range []string{"plugin", "component", "includes", "filterscript"} {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()

			pcx := lifecycleContext(t, "json")
			pcx.Package.Dependencies = []versioning.DependencyString{versioning.DependencyString(fmt.Sprintf("%s://fixture/parent:1.0.0", scheme))}
			require.NoError(t, pcx.Package.WriteDefinition())
			parent := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "parent", Tag: "1.0.0"}
			child := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "child", Tag: "1.0.0"}
			seedReliabilityRepo(t, parent.CachePath(pcx.CacheDir), `{"dependencies":["fixture/child:1.0.0"],"resources":[{"name":"parent.so","platform":"linux"}]}`)
			seedReliabilityRepo(t, child.CachePath(pcx.CacheDir), `{}`)
			require.NoError(t, pcx.InitLockfileResolver("test"))

			_, err := pcx.EnsureProject(t.Context(), DependencyUpdateRequest{})
			require.NoError(t, err)
			require.Len(t, pcx.AllDependencies, 2)
			if scheme == "plugin" || scheme == "component" {
				require.Len(t, pcx.AllPlugins, 1)
				require.Equal(t, scheme, pcx.AllPlugins[0].Scheme)
			} else {
				require.Empty(t, pcx.AllPlugins)
			}
			require.FileExists(t, filepath.Join(pcx.Package.Vendor, "child", "pawn.json"))
			locked, err := lockfile.Load(pcx.Package.LocalPath)
			require.NoError(t, err)
			require.Equal(t, 2, locked.DependencyCount())
			entry, ok := locked.GetDependency(lockfile.DependencyKey(child))
			require.True(t, ok)
			require.True(t, entry.Transitive)
			require.Equal(t, []string{lockfile.DependencyKey(parent)}, entry.RequiredBy)
		})
	}
}
