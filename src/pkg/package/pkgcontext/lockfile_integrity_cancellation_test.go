package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

type cancelAfterFirstCheckContext struct {
	context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (ctx *cancelAfterFirstCheckContext) Err() error {
	err := ctx.Context.Err()
	ctx.once.Do(ctx.cancel)
	return err
}

func TestLockedDependencyIntegrityRetainsCancellation(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "include.inc"), []byte("fixture"), 0o600))
	integrity, err := lockfile.CalculateDirectoryIntegrity(directory)
	require.NoError(t, err)
	meta := versioning.DependencyMeta{User: "fixture", Repo: "shared"}
	locked := lockfile.New("test")
	locked.AddDependency(lockfile.DependencyKey(meta), lockfile.LockedDependency{Integrity: integrity})
	pcx := PackageContext{PackageLockfileState: PackageLockfileState{lockfileResolver: &fakeDependencyLock{lockfile: locked}}}
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &cancelAfterFirstCheckContext{Context: base, cancel: cancel}

	err = pcx.verifyDependencyIntegrityWithRecoveryContext(ctx, meta, directory, false)
	require.ErrorIs(t, err, context.Canceled)
}
