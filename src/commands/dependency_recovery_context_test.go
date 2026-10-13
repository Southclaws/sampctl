package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirsle/configdir"

	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestLoadDependencyOperationContextRecoversBeforeReadingManifest(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	root, _ := newCheckRecoveryFixture(t, "active")
	if err := os.Remove(filepath.Join(root, "pawn.json")); err != nil {
		t.Fatal(err)
	}
	ctx := newTestCLIContext("test")
	pcx, _, err := loadDependencyOperationContext(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if pcx.Package.User != "before" {
		t.Errorf("loaded user = %q, want captured manifest user", pcx.Package.User)
	}
	if _, err := os.Lstat(filepath.Join(root, pkgcontext.DependencyRecoveryFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("recovered context retained metadata: %v", err)
	}
}

func TestLoadDependencyOperationContextDoesNotRecoverParent(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	root, _ := newCheckRecoveryFixture(t, "active")
	child := filepath.Join(root, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx := newTestCLIContext("test")
	if _, _, err := loadDependencyOperationContext(ctx, child); err != nil {
		t.Fatal(err)
	}
	if got := string(readCheckRecoveryFixture(t, filepath.Join(root, "pawn.json"))); got != `{"user":"after"}` {
		t.Errorf("loading nested directory restored parent manifest: %s", got)
	}
	if _, err := os.Lstat(filepath.Join(root, pkgcontext.DependencyRecoveryFilename)); err != nil {
		t.Errorf("loading nested directory removed parent's recovery state: %v", err)
	}
}
