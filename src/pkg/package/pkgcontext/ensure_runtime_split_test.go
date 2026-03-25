package pkgcontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

type fakeRuntimeProvisioner struct {
	layoutCalled   bool
	binariesCalled bool
	pluginsCalled  bool
	workingDir     string
	isOpenMP       bool
	cacheDir       string
	config         runtimecfg.Runtime
	pluginsReq     runtime.EnsurePluginsRequest
	manifestInfo   *runtime.RuntimeManifestInfo
	layoutErr      error
	binariesErr    error
	pluginsErr     error
}

func (f *fakeRuntimeProvisioner) EnsurePackageLayout(workingDir string, isOpenMP bool) error {
	f.layoutCalled = true
	f.workingDir = workingDir
	f.isOpenMP = isOpenMP
	return f.layoutErr
}

func (f *fakeRuntimeProvisioner) EnsureBinaries(_ context.Context, cacheDir string, cfg runtimecfg.Runtime) (*runtime.RuntimeManifestInfo, error) {
	f.binariesCalled = true
	f.cacheDir = cacheDir
	f.config = cfg
	return f.manifestInfo, f.binariesErr
}

func (f *fakeRuntimeProvisioner) EnsurePlugins(request runtime.EnsurePluginsRequest) error {
	f.pluginsCalled = true
	f.pluginsReq = request
	return f.pluginsErr
}

func TestEnsureParentRuntimeUsesInjectedRuntimeProvisioner(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	data, err := json.Marshal(map[string]any{
		"runtime": map[string]any{"version": "0.3.7"},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), data, 0o644))

	resolver := &fakeDependencyLock{lockfile: lockfile.New("dev")}
	provisioner := &fakeRuntimeProvisioner{
		manifestInfo: &runtime.RuntimeManifestInfo{
			Version:     "0.3.7",
			Platform:    "linux",
			RuntimeType: "samp",
			Files:       []runtime.RuntimeFileInfo{{Path: "server", Size: 128, Hash: "abc123", Mode: 0o755}},
		},
	}

	pcx, err := NewPackageContext(NewPackageContextOptions{
		Parent:      true,
		Dir:         projectDir,
		Platform:    "linux",
		CacheDir:    t.TempDir(),
		RuntimeProv: provisioner,
	})
	require.NoError(t, err)
	pcx.AllPlugins = []versioning.DependencyMeta{{Repo: "test-plugin", Scheme: "plugin"}}
	pcx.PackageLockfileState.SetLockfileResolver(resolver)

	err = pcx.ensureParentRuntime(context.Background())
	require.NoError(t, err)
	assert.True(t, provisioner.layoutCalled)
	assert.True(t, provisioner.binariesCalled)
	assert.True(t, provisioner.pluginsCalled)
	assert.Equal(t, projectDir, provisioner.workingDir)
	assert.Equal(t, pcx.CacheDir, provisioner.cacheDir)
	assert.Equal(t, []versioning.DependencyMeta{{Repo: "test-plugin", Scheme: "plugin"}}, provisioner.config.PluginDeps)
	assert.Equal(t, &pcx.ActualRuntime, provisioner.pluginsReq.Config)
	assert.True(t, resolver.saved)
	assert.Equal(t, "0.3.7", resolver.runtimeVersion)
	assert.Equal(t, "linux", resolver.runtimePlatform)
	assert.Equal(t, []lockfile.LockedFileInfo{{Path: "server", Size: 128, Hash: "abc123", Mode: 0o755}}, resolver.runtimeFiles)
}

func TestRecordRuntimeToLockfileCopiesManifestFiles(t *testing.T) {
	t.Parallel()

	resolver := &fakeDependencyLock{lockfile: lockfile.New("dev")}
	pcx := &PackageContext{
		PackageResolvedState: PackageResolvedState{
			ActualRuntime: runtimecfg.Runtime{Version: "omp", Platform: "linux", RuntimeType: runtimecfg.RuntimeTypeOpenMP},
		},
		PackageLockfileState: PackageLockfileState{lockfileResolver: resolver},
	}

	pcx.recordRuntimeToLockfile(&runtime.RuntimeManifestInfo{
		Version:     "omp",
		Platform:    "linux",
		RuntimeType: "openmp",
		Files:       []runtime.RuntimeFileInfo{{Path: "server", Size: 64, Hash: "abc", Mode: 0o755}},
	})

	assert.Equal(t, "omp", resolver.runtimeVersion)
	assert.Equal(t, "linux", resolver.runtimePlatform)
	assert.Equal(t, "openmp", resolver.runtimeType)
	assert.Equal(t, []lockfile.LockedFileInfo{{Path: "server", Size: 64, Hash: "abc", Mode: 0o755}}, resolver.runtimeFiles)
}

func TestRecordBuildToLockfileHashesOutput(t *testing.T) {
	t.Parallel()

	resolver := &fakeDependencyLock{lockfile: lockfile.New("dev")}
	output := filepath.Join(t.TempDir(), "test.amx")
	contents := []byte("hello world")
	require.NoError(t, os.WriteFile(output, contents, 0o644))

	pcx := &PackageContext{PackageLockfileState: PackageLockfileState{lockfileResolver: resolver}}
	pcx.RecordBuildToLockfile("1.0.0", "default", "src/main.pwn", output)

	expectedHash := sha256.Sum256(contents)
	assert.Equal(t, lockfile.BuildRecord{
		CompilerVersion: "1.0.0",
		CompilerPreset:  "default",
		Entry:           "src/main.pwn",
		Output:          output,
		OutputHash:      "sha256:" + hex.EncodeToString(expectedHash[:]),
	}, resolver.buildRecord)
}

func TestRecordBuildToLockfileNoResolverIsNoOp(t *testing.T) {
	t.Parallel()

	pcx := &PackageContext{}
	assert.NotPanics(t, func() {
		pcx.RecordBuildToLockfile("1.0.0", "default", "entry", "")
	})
}
