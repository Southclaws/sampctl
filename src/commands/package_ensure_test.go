package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

type fakeEnsureCommandTarget struct {
	fakeCommandLockfile
	ensureCalled      bool
	ensureForceUpdate bool
	ensureUpdated     bool
	ensureErr         error
}

func (f *fakeEnsureCommandTarget) EnsureProject(_ context.Context, forceUpdate bool) (bool, error) {
	f.ensureCalled = true
	f.ensureForceUpdate = forceUpdate
	return f.ensureUpdated, f.ensureErr
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
		forceUpdate: true,
		useLockfile: true,
		lockOnly:    false,
	})
	require.NoError(t, err)
	assert.Equal(t, "dev", target.initVersion)
	assert.True(t, target.forceUpdateCalled)
	assert.True(t, target.ensureCalled)
	assert.True(t, target.ensureForceUpdate)
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
		forceUpdate: false,
		useLockfile: true,
		lockOnly:    true,
	})
	require.NoError(t, err)
	assert.False(t, target.ensureCalled)
	assert.True(t, target.saved)
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
