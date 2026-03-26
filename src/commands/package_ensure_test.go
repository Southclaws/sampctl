package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

type fakeEnsureCommandTarget struct {
	fakeCommandLockfile
	ensureCalled          bool
	ensureRequest         pkgcontext.DependencyUpdateRequest
	ensureUpdated         bool
	ensureErr             error
	updateLockfileCalled  bool
	updateLockfileRequest pkgcontext.DependencyUpdateRequest
	updateLockfileErr     error
}

func (f *fakeEnsureCommandTarget) EnsureProject(_ context.Context, request pkgcontext.DependencyUpdateRequest) (bool, error) {
	f.ensureCalled = true
	f.ensureRequest = request
	return f.ensureUpdated, f.ensureErr
}

func (f *fakeEnsureCommandTarget) UpdateLockfile(_ context.Context, request pkgcontext.DependencyUpdateRequest) error {
	f.updateLockfileCalled = true
	f.updateLockfileRequest = request
	return f.updateLockfileErr
}

func TestRunPackageEnsureForceUpdate(t *testing.T) {
	t.Parallel()

	target := &fakeEnsureCommandTarget{
		fakeCommandLockfile: fakeCommandLockfile{
			hasLockfile: true,
			hasResolver: true,
			lockfile:    lockfile.New("dev"),
		},
	}

	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{
		version:     "dev",
		useLockfile: true,
		lockOnly:    false,
		update: pkgcontext.DependencyUpdateRequest{
			Enabled: true,
			Force:   true,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "dev", target.initVersion)
	assert.True(t, target.forceUpdateCalled)
	assert.True(t, target.ensureCalled)
	assert.True(t, target.ensureRequest.Enabled)
	assert.True(t, target.ensureRequest.Force)
}

func TestRunPackageEnsureTargetedForceDoesNotClearWholeLockfile(t *testing.T) {
	t.Parallel()

	targetSelector := "user/repo"
	targetMeta, err := versioning.DependencyString(targetSelector).Explode()
	require.NoError(t, err)

	target := &fakeEnsureCommandTarget{
		fakeCommandLockfile: fakeCommandLockfile{
			hasLockfile: true,
			hasResolver: true,
			lockfile:    lockfile.New("dev"),
		},
	}

	err = runPackageEnsure(context.Background(), target, ensureCommandOptions{
		version:     "dev",
		useLockfile: true,
		update: pkgcontext.DependencyUpdateRequest{
			Enabled:    true,
			Force:      true,
			Target:     targetSelector,
			TargetMeta: targetMeta,
		},
	})
	require.NoError(t, err)
	assert.False(t, target.forceUpdateCalled)
	assert.True(t, target.ensureCalled)
	assert.Equal(t, targetMeta, target.ensureRequest.TargetMeta)
}

func TestRunPackageEnsureLockOnlySkipsEnsure(t *testing.T) {
	t.Parallel()

	target := &fakeEnsureCommandTarget{
		fakeCommandLockfile: fakeCommandLockfile{
			hasResolver: true,
		},
	}

	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{
		version:     "dev",
		useLockfile: true,
		lockOnly:    true,
		update:      pkgcontext.DependencyUpdateRequest{},
	})
	require.NoError(t, err)
	assert.False(t, target.ensureCalled)
	assert.True(t, target.updateLockfileCalled)
	assert.False(t, target.updateLockfileRequest.Force)
	assert.True(t, target.saved)
}

func TestRunPackageEnsureLockOnlyReturnsUpdateError(t *testing.T) {
	t.Parallel()

	target := &fakeEnsureCommandTarget{
		fakeCommandLockfile: fakeCommandLockfile{hasResolver: true},
		updateLockfileErr:   errors.New("boom"),
	}

	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{
		version:     "dev",
		useLockfile: true,
		lockOnly:    true,
		update: pkgcontext.DependencyUpdateRequest{
			Enabled: true,
			Force:   true,
		},
	})
	require.Error(t, err)
	assert.EqualError(t, err, "failed to update lockfile: boom")
	assert.True(t, target.updateLockfileCalled)
	assert.True(t, target.updateLockfileRequest.Force)
	assert.False(t, target.ensureCalled)
}

func TestRunPackageEnsureReturnsEnsureError(t *testing.T) {
	t.Parallel()

	target := &fakeEnsureCommandTarget{
		ensureErr: errors.New("boom"),
	}

	err := runPackageEnsure(context.Background(), target, ensureCommandOptions{})
	require.Error(t, err)
	assert.EqualError(t, err, "failed to ensure dependencies: boom")
	assert.True(t, target.ensureCalled)
}
