package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

// AdoptRuntimeOwnership records a legacy installation only when every file still
// matches the previous project lockfile and no installed ownership record exists.
func AdoptRuntimeOwnership(ctx context.Context, root string, previous *lockfile.LockedRuntime) (adopted bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if previous == nil {
		return false, nil
	}
	path := filepath.Join(root, installedRuntimeManifestFileName)
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	manifest := runtimeManifestFromLockedRuntime(previous)
	if validateInstalledRuntimeManifest(manifest, root) != nil || verifyRuntimeManifest(manifest, root) != nil {
		return false, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(root, ".sampctl-runtime-adopt-")
	if err != nil {
		return false, err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, temporary.Close())
		}
		err = errors.Join(err, os.Remove(temporary.Name()))
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return false, err
	}
	if _, err := temporary.Write(data); err != nil {
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		return false, err
	}
	err = temporary.Close()
	closed = true
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Linking publishes the complete record without replacing one created by
	// another project that points at the same external runtime directory.
	if err := os.Link(temporary.Name(), path); errors.Is(err, os.ErrExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("publish adopted runtime ownership: %w", err)
	}
	return true, fs.SyncDir(root)
}
