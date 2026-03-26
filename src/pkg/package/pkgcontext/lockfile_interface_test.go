package pkgcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

type fakeDependencyLock struct {
	lockfile         *lockfile.Lockfile
	previous         lockfile.LockedDependency
	hasPrevious      bool
	saved            bool
	forced           bool
	hasLocked        bool
	lockedVersion    versioning.DependencyMeta
	lastResolution   lockfile.DependencyResolution
	lastResolutionIn versioning.DependencyMeta
	lastTransitive   bool
	lastRequiredBy   string
	runtimeVersion   string
	runtimePlatform  string
	runtimeType      string
	runtimeFiles     []lockfile.LockedFileInfo
	buildRecord      lockfile.BuildRecord
}

func (f *fakeDependencyLock) GetLockedVersion(meta versioning.DependencyMeta) versioning.DependencyMeta {
	if f.lockedVersion.Repo != "" || f.lockedVersion.Tag != "" || f.lockedVersion.Commit != "" || f.lockedVersion.Branch != "" {
		return f.lockedVersion
	}
	return meta
}

func (f *fakeDependencyLock) RecordResolution(meta versioning.DependencyMeta, resolution lockfile.DependencyResolution, transitive bool, requiredBy string) error {
	f.lastResolutionIn = meta
	f.lastResolution = resolution
	f.lastTransitive = transitive
	f.lastRequiredBy = requiredBy
	return nil
}

func (f *fakeDependencyLock) GetPreviousDependency(versioning.DependencyMeta) (lockfile.LockedDependency, bool) {
	if !f.hasPrevious {
		return lockfile.LockedDependency{}, false
	}
	return f.previous, true
}

func (f *fakeDependencyLock) RecordLocalDependency(versioning.DependencyMeta) error {
	return nil
}

func (f *fakeDependencyLock) RecordRuntime(version, platform, runtimeType string, files []lockfile.LockedFileInfo) {
	f.runtimeVersion = version
	f.runtimePlatform = platform
	f.runtimeType = runtimeType
	f.runtimeFiles = files
}

func (f *fakeDependencyLock) RecordBuild(record lockfile.BuildRecord) {
	f.buildRecord = record
}

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
	pcx := &PackageContext{PackageLockfileState: PackageLockfileState{lockfileResolver: fake}}

	require.NoError(t, pcx.SaveLockfile())
	assert.True(t, fake.saved)
	assert.True(t, pcx.HasLockfile())
	assert.True(t, pcx.HasLockfileResolver())
	assert.NotNil(t, pcx.GetLockfile())

	pcx.ForceUpdateLockfile()
	assert.True(t, fake.forced)

	pcx.PackageLockfileState.lockfileResolver = nil
	assert.NoError(t, pcx.SaveLockfile())
	assert.False(t, pcx.HasLockfile())
	assert.False(t, pcx.HasLockfileResolver())
	assert.Nil(t, pcx.GetLockfile())
}
