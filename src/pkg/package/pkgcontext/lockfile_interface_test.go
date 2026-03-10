package pkgcontext

import (
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

type fakeDependencyLock struct {
	lockfile  *lockfile.Lockfile
	saved     bool
	forced    bool
	hasLocked bool
}

func (f *fakeDependencyLock) GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta {
	return meta
}

func (f *fakeDependencyLock) RecordResolution(versioning.DependencyMeta, *git.Repository, bool, string) error {
	return nil
}

func (f *fakeDependencyLock) RecordLocalDependency(versioning.DependencyMeta) error {
	return nil
}

func (f *fakeDependencyLock) RecordRuntime(string, string, string, []lockfile.LockedFileInfo) {}

func (f *fakeDependencyLock) RecordBuild(string, string, string, string, string) {}

func (f *fakeDependencyLock) Save() error {
	f.saved = true
	return nil
}

func (f *fakeDependencyLock) ForceUpdate() {
	f.forced = true
}

func (f *fakeDependencyLock) HasLockfile() bool {
	return f.hasLocked
}

func (f *fakeDependencyLock) GetLockfile() *lockfile.Lockfile {
	return f.lockfile
}

func TestPackageContextLockfileInterfaceHelpers(t *testing.T) {
	t.Parallel()

	fake := &fakeDependencyLock{
		lockfile:  lockfile.New("dev"),
		hasLocked: true,
	}
	pcx := &PackageContext{lockfileResolver: fake}

	require.NoError(t, pcx.SaveLockfile())
	assert.True(t, fake.saved)
	assert.True(t, pcx.HasLockfile())
	assert.True(t, pcx.HasLockfileResolver())
	assert.NotNil(t, pcx.GetLockfile())

	pcx.ForceUpdateLockfile()
	assert.True(t, fake.forced)

	pcx.lockfileResolver = nil
	assert.NoError(t, pcx.SaveLockfile())
	assert.False(t, pcx.HasLockfile())
	assert.False(t, pcx.HasLockfileResolver())
	assert.Nil(t, pcx.GetLockfile())
}
