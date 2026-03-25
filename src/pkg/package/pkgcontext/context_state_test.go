package pkgcontext

import (
	"testing"

	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func TestPackageServicesResolvers(t *testing.T) {
	t.Parallel()

	store := &fakeRepositoryStore{}
	health := &fakeRepositoryHealth{}
	runtimeEnv := constructorRuntimeEnvironment{}
	gh := github.NewClient(nil)

	services := PackageServices{
		GitHub:     gh,
		RepoStore:  store,
		RepoHealth: health,
		RuntimeEnv: runtimeEnv,
	}

	assert.Same(t, store, services.repositoryStore())
	assert.Same(t, health, services.repositoryHealth())
	assert.Equal(t, runtimeEnv, services.runtimeEnvironment())
	assert.Same(t, gh, services.GitHub)

	defaults := PackageServices{}
	assert.IsType(t, GitRepositoryStore{}, defaults.repositoryStore())
	assert.IsType(t, GitRepositoryHealth{}, defaults.repositoryHealth())
	assert.IsType(t, runtimeEnvironmentAdapter{}, defaults.runtimeEnvironment())
}

func TestPackageLockfileStateMethods(t *testing.T) {
	t.Parallel()

	fake := &fakeDependencyLock{
		lockfile:  lockfile.New("dev"),
		hasLocked: true,
	}
	state := PackageLockfileState{lockfileResolver: fake}

	require.NoError(t, state.SaveLockfile())
	assert.True(t, fake.saved)
	assert.True(t, state.HasLockfile())
	assert.True(t, state.HasLockfileResolver())
	assert.NotNil(t, state.GetLockfile())

	state.ForceUpdateLockfile()
	assert.True(t, fake.forced)

	state.lockfileResolver = nil
	assert.NoError(t, state.SaveLockfile())
	assert.False(t, state.HasLockfile())
	assert.False(t, state.HasLockfileResolver())
	assert.Nil(t, state.GetLockfile())
}
