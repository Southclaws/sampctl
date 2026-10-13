package pkgcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsureParentRuntimeStopsAtCanceledProvisioningBoundary(t *testing.T) {
	for _, phase := range []string{"layout", "binaries", "plugins"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			provisioner := &cancelingRuntimeProvisioner{phase: phase, cancel: cancel}
			project := t.TempDir()
			if err := os.WriteFile(filepath.Join(project, "pawn.json"), []byte(`{"runtime":{"version":"0.3.7"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			pcx, err := LoadPackageContext(NewPackageContextOptions{Dir: project, Parent: true, CacheDir: t.TempDir(), Platform: "linux", RuntimeProv: provisioner})
			if err != nil {
				t.Fatal(err)
			}
			resolver := &fakeDependencyLock{lockfile: lockfile.New("test")}
			pcx.PackageLockfileState.SetLockfileResolver(resolver)
			if err := pcx.ensureParentRuntime(ctx); !errors.Is(err, context.Canceled) {
				t.Errorf("ensureParentRuntime(cancel after %s) = %v, want context.Canceled", phase, err)
			}
			if resolver.saved {
				t.Error("ensureParentRuntime(canceled) saved lockfile, want no persistence")
			}
			wantCalls := map[string]int{"layout": 1, "binaries": 2, "plugins": 3}[phase]
			if provisioner.calls != wantCalls {
				t.Errorf("ensureParentRuntime(cancel after %s) called %d provisioning phases, want %d", phase, provisioner.calls, wantCalls)
			}
		})
	}
}

type cancelingRuntimeProvisioner struct {
	phase  string
	cancel context.CancelFunc
	calls  int
}

func (p *cancelingRuntimeProvisioner) finishPhase(phase string) {
	p.calls++
	if phase == p.phase {
		p.cancel()
	}
}

func (p *cancelingRuntimeProvisioner) EnsurePackageLayout(string, bool) error {
	p.finishPhase("layout")
	return nil
}

func (p *cancelingRuntimeProvisioner) EnsureBinaries(context.Context, string, runtimecfg.Runtime) (*runtimepkg.RuntimeManifestInfo, error) {
	p.finishPhase("binaries")
	return &runtimepkg.RuntimeManifestInfo{Version: "0.3.7", Platform: "linux"}, nil
}

func (p *cancelingRuntimeProvisioner) EnsurePlugins(runtimepkg.EnsurePluginsRequest) error {
	p.finishPhase("plugins")
	return nil
}

func TestEnsureParentRuntimeRejectsSymlinkedRuntimeAncestor(t *testing.T) {
	project, external := t.TempDir(), t.TempDir()
	if err := os.Symlink(external, filepath.Join(project, "server")); err != nil {
		t.Skipf("creating symlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "pawn.json"), []byte(`{"runtime_dir":"server/nested","runtime":{"version":"0.3.7"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	provisioner := &fakeRuntimeProvisioner{}
	pcx, err := LoadPackageContext(NewPackageContextOptions{Dir: project, Parent: true, CacheDir: t.TempDir(), Platform: "linux", RuntimeProv: provisioner})
	if err != nil {
		t.Fatal(err)
	}
	if err := pcx.ensureParentRuntime(context.Background()); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("ensureParentRuntime(symlinked ancestor) = %v, want os.ErrInvalid", err)
	}
	if provisioner.layoutCalled || provisioner.binariesCalled || provisioner.pluginsCalled {
		t.Error("ensureParentRuntime(symlinked ancestor) called provisioner, want refusal before mutation")
	}
	entries, err := os.ReadDir(external)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("ensureParentRuntime(symlinked ancestor) left %v externally, want no files", entries)
	}
}
