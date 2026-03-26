package pkgcontext

import (
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func TestPackageServicesResolvers(t *testing.T) {
	t.Parallel()

	store := &fakeRepositoryStore{}
	health := &fakeRepositoryHealth{}
	runtimeEnv := constructorRuntimeEnvironment{}
	runtimeProv := constructorRuntimeProvisioner{}
	gh := github.NewClient(nil)

	services := PackageServices{
		GitHub:      gh,
		RepoStore:   store,
		RepoHealth:  health,
		RuntimeEnv:  runtimeEnv,
		RuntimeProv: runtimeProv,
	}

	assert.Same(t, store, services.repositoryStore())
	assert.Same(t, health, services.repositoryHealth())
	assert.Equal(t, runtimeEnv, services.runtimeEnvironment())
	assert.Equal(t, runtimeProv, services.runtimeProvisioner())
	assert.Same(t, gh, services.GitHub)

	defaults := PackageServices{}
	assert.IsType(t, GitRepositoryStore{}, defaults.repositoryStore())
	assert.IsType(t, GitRepositoryHealth{}, defaults.repositoryHealth())
	assert.IsType(t, runtimeEnvironmentAdapter{}, defaults.runtimeEnvironment())
	assert.IsType(t, runtimeProvisionerAdapter{}, defaults.runtimeProvisioner())
}

func TestPackageLockfileStateMethods(t *testing.T) {
	t.Parallel()

	fake := &fakeDependencyLock{
		lockfile:      lockfile.New("dev"),
		hasLocked:     true,
		lockedVersion: versioning.DependencyMeta{Repo: "locked", Tag: "v1.0.0"},
	}
	state := PackageLockfileState{lockfileResolver: fake}
	meta := versioning.DependencyMeta{Repo: "original", Tag: "v0.1.0"}
	resolution := lockfile.DependencyResolution{Resolved: "v1.0.0", Commit: "abc123"}
	files := []lockfile.LockedFileInfo{{Path: "server", Size: 42, Hash: "sha256:abc", Mode: 0o755}}
	record := lockfile.BuildRecord{CompilerVersion: "1.0.0", Output: "gamemodes/test.amx"}

	assert.Equal(t, fake.lockedVersion, state.LockedVersion(meta, false))
	assert.Equal(t, meta, state.LockedVersion(meta, true))
	_, ok := state.PreviousDependency(meta)
	assert.False(t, ok)
	require.NoError(t, state.RecordDependencyResolution(meta, resolution, true, "parent/repo"))
	state.RecordRuntime("1.2.3", "linux", "server", files)
	state.RecordBuild(record)

	require.NoError(t, state.SaveLockfile())
	assert.True(t, fake.saved)
	assert.Equal(t, meta, fake.lastResolutionIn)
	assert.Equal(t, resolution, fake.lastResolution)
	assert.True(t, fake.lastTransitive)
	assert.Equal(t, "parent/repo", fake.lastRequiredBy)
	assert.Equal(t, "1.2.3", fake.runtimeVersion)
	assert.Equal(t, "linux", fake.runtimePlatform)
	assert.Equal(t, "server", fake.runtimeType)
	assert.Equal(t, files, fake.runtimeFiles)
	assert.Equal(t, record, fake.buildRecord)
	assert.True(t, state.HasLockfile())
	assert.True(t, state.HasLockfileResolver())
	assert.NotNil(t, state.GetLockfile())

	state.ForceUpdateLockfile()
	assert.True(t, fake.forced)

	state.lockfileResolver = nil
	assert.Equal(t, meta, state.LockedVersion(meta, false))
	_, ok = state.PreviousDependency(meta)
	assert.False(t, ok)
	assert.NoError(t, state.RecordDependencyResolution(meta, resolution, false, ""))
	state.RecordRuntime("1.2.3", "linux", "server", files)
	state.RecordBuild(record)
	assert.NoError(t, state.SaveLockfile())
	assert.False(t, state.HasLockfile())
	assert.False(t, state.HasLockfileResolver())
	assert.Nil(t, state.GetLockfile())
}
