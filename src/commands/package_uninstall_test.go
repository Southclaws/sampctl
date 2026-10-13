package commands

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestPackageUninstallUnavailableDependencyWithoutTouchingCache(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	cacheDir, err := fs.ConfigDir()
	require.NoError(t, err)
	relative, err := filepath.Rel(configHome, cacheDir)
	require.NoError(t, err)
	require.True(t, filepath.IsLocal(relative), "cache must remain inside the temporary home")
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "keep.txt"), []byte("shared cache data"), 0o600))

	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "repository unavailable", http.StatusNotFound)
	}))
	defer server.Close()
	dependency := versioning.DependencyString(server.URL + "/fixture/missing:1.0.0")

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			pkg := pawnpackage.Package{Format: format, LocalPath: dir, User: "fixture", Repo: "project", Dependencies: []versioning.DependencyString{dependency}}
			require.NoError(t, pkg.WriteDefinition())
			meta, err := dependency.Explode()
			require.NoError(t, err)
			lf := lockfile.New("dev")
			lf.AddDependency(lockfile.DependencyKey(meta), lockfile.LockedDependency{Site: meta.Site, User: meta.User, Repo: meta.Repo, Constraint: ":1.0.0", Commit: "0123456789012345678901234567890123456789"})
			require.NoError(t, lockfile.Save(dir, lf))
			installed := filepath.Join(dir, "dependencies", "missing")
			require.NoError(t, os.MkdirAll(installed, 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(installed, "keep.inc"), []byte("user installation"), 0o600))
			beforeCache := snapshotCheckDependency(t, cacheDir)
			beforeInstalled := snapshotCheckDependency(t, installed)
			state := newCommandState("dev", cacheDir)
			app := newCLIApp("dev", state)
			app.Writer, app.ErrWriter = io.Discard, io.Discard
			set := flag.NewFlagSet("uninstall", flag.ContinueOnError)
			for _, cliFlag := range withGlobalFlags(globalFlags(), packageUninstallFlags()) {
				cliFlag.Apply(set)
			}
			require.NoError(t, set.Parse([]string{"--dir", dir, string(dependency)}))
			ctx := cli.NewContext(app, set, nil)

			require.NoError(t, packageUninstall(ctx))

			updated, err := pawnpackage.PackageFromDir(dir)
			require.NoError(t, err)
			assert.Empty(t, updated.Dependencies)
			updatedLock, err := lockfile.Load(dir)
			require.NoError(t, err)
			require.NotNil(t, updatedLock)
			assert.Empty(t, updatedLock.Dependencies)
			assert.Equal(t, beforeCache, snapshotCheckDependency(t, cacheDir))
			assert.Equal(t, beforeInstalled, snapshotCheckDependency(t, installed))
			assert.Zero(t, requests.Load(), "uninstall must not contact the repository")
		})
	}
}
