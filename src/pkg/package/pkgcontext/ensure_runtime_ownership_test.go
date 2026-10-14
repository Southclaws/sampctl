package pkgcontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
)

func TestEnsureParentRuntimeAdoptsLegacyExternalInstallation(t *testing.T) {
	pcx, external, binary, _ := legacyExternalRuntimeContext(t)

	if _, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{}); err != nil {
		t.Fatalf("EnsureProject(legacy external runtime) = %v, want upgrade", err)
	}

	if got := string(readCrashRecoveryFile(t, filepath.Join(external, binary))); got != "replacement runtime" {
		t.Errorf("external runtime after upgrade = %q, want replacement runtime", got)
	}

	info, err := runtimepkg.GetRuntimeManifestInfo(external)
	if err != nil || info == nil || info.Version != "0.3.7" {
		t.Errorf("GetRuntimeManifestInfo(upgraded external runtime) = %+v, %v; want recorded version 0.3.7", info, err)
	}

	lf, err := lockfile.Load(pcx.Package.LocalPath)
	if err != nil || lf == nil || lf.Runtime == nil || lf.Runtime.Version != "0.3.7" {
		t.Errorf("project lockfile after external upgrade = %+v, %v; want runtime version 0.3.7", lf, err)
	}
}

func TestEnsureParentRuntimeDoesNotAdoptModifiedExternalInstallation(t *testing.T) {
	pcx, external, binary, _ := legacyExternalRuntimeContext(t)
	target := filepath.Join(external, binary)
	if err := os.WriteFile(target, []byte("user runtime changes"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{}); !errors.Is(err, os.ErrExist) {
		t.Errorf("EnsureProject(modified external runtime) = %v, want untracked conflict", err)
	}

	if got := string(readCrashRecoveryFile(t, target)); got != "user runtime changes" {
		t.Errorf("external runtime after refused adoption = %q, want user runtime changes", got)
	}

	if _, err := os.Lstat(filepath.Join(external, ".sampctl-runtime.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("refused legacy adoption created ownership metadata: %v", err)
	}
}

func TestRuntimeAdoptionPreservesExistingOwnershipAndCancellation(t *testing.T) {
	_, external, binary, previous := legacyExternalRuntimeContext(t)
	path := filepath.Join(external, ".sampctl-runtime.json")
	if err := os.WriteFile(path, []byte("existing ownership record"), 0o600); err != nil {
		t.Fatal(err)
	}

	adopted, err := runtimepkg.AdoptRuntimeOwnership(context.Background(), external, previous)
	if err != nil || adopted {
		t.Errorf("AdoptRuntimeOwnership(existing sidecar) = %v, %v; want false, nil", adopted, err)
	}

	if got := string(readCrashRecoveryFile(t, path)); got != "existing ownership record" {
		t.Errorf("existing runtime ownership = %q, want original record", got)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if adopted, err := runtimepkg.AdoptRuntimeOwnership(ctx, external, previous); adopted || !errors.Is(err, context.Canceled) {
		t.Errorf("AdoptRuntimeOwnership(canceled) = %v, %v; want false, context.Canceled", adopted, err)
	}

	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("canceled adoption created ownership metadata: %v", err)
	}

	if got := string(readCrashRecoveryFile(t, filepath.Join(external, binary))); got != "previous runtime" {
		t.Errorf("runtime after canceled adoption = %q, want previous runtime", got)
	}
}

func TestEnsureParentRuntimeSkipsAdoptionForInjectedProvisioner(t *testing.T) {
	pcx, external, _, _ := legacyExternalRuntimeContext(t)
	provisioner := &fakeRuntimeProvisioner{}
	pcx.RuntimeProv = provisioner
	pcx.PackageLockfileState.SetLockfileResolver(&fakeDependencyLock{lockfile: lockfile.New("test")})
	if err := os.WriteFile(filepath.Join(pcx.Package.LocalPath, lockfile.Filename), []byte("unreadable legacy record"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := pcx.ensureParentRuntime(context.Background()); err != nil {
		t.Fatalf("ensureParentRuntime(injected provisioner) = %v, want no adoption I/O", err)
	}

	if !provisioner.layoutCalled || !provisioner.binariesCalled || !provisioner.pluginsCalled {
		t.Error("injected runtime provisioner did not complete its normal phases")
	}

	if _, err := os.Lstat(filepath.Join(external, ".sampctl-runtime.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("injected provisioner caused ownership adoption: %v", err)
	}
}

func legacyExternalRuntimeContext(t *testing.T) (*PackageContext, string, string, *lockfile.LockedRuntime) {
	t.Helper()

	project, external, cache := t.TempDir(), t.TempDir(), t.TempDir()
	platform, binary := runtime.GOOS, "samp03svr"
	if platform == "windows" {
		binary = "samp-server.exe"
	}

	definition, err := json.Marshal(map[string]any{
		"runtime_dir": external,
		"runtime":     map[string]string{"version": "0.3.7"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "pawn.json"), definition, 0o644); err != nil {
		t.Fatal(err)
	}

	oldFile := writeOwnershipRuntimeFixture(t, filepath.Join(external, binary), "previous runtime")
	oldFile.Path = binary
	previous := &lockfile.LockedRuntime{Version: "0.3.DL", Platform: platform, RuntimeType: "samp", Files: []lockfile.LockedFileInfo{oldFile}}
	lf := lockfile.New("test")
	lf.Runtime = previous
	if err := lockfile.Save(project, lf); err != nil {
		t.Fatal(err)
	}

	stage := filepath.Join(cache, "runtime_staging", platform, "0.3.7")
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}

	newFile := writeOwnershipRuntimeFixture(t, filepath.Join(stage, binary), "replacement runtime")
	newFile.Path = binary
	staged, err := json.Marshal(map[string]any{
		"version":      "0.3.7",
		"platform":     platform,
		"runtime_type": "samp",
		"files":        []lockfile.LockedFileInfo{newFile},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(stage, "sampctl-runtime-manifest.json"), staged, 0o600); err != nil {
		t.Fatal(err)
	}

	pcx, err := LoadPackageContext(NewPackageContextOptions{Parent: true, Dir: project, CacheDir: cache, Platform: platform})
	if err != nil {
		t.Fatal(err)
	}

	if err := pcx.InitLockfileResolver("test"); err != nil {
		t.Fatal(err)
	}

	return pcx, external, binary, previous
}

func writeOwnershipRuntimeFixture(t *testing.T, path, contents string) lockfile.LockedFileInfo {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	hash := sha256.Sum256([]byte(contents))
	return lockfile.LockedFileInfo{Size: info.Size(), Hash: hex.EncodeToString(hash[:]), Mode: uint32(info.Mode().Perm())}
}
