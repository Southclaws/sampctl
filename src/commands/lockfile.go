package commands

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func initLockfileResolver(c *cli.Context, target pkgcontext.LockfileInitializer) error {
	state, err := getCommandState(c)
	if err != nil {
		return err
	}
	return target.InitLockfileResolver(state.version)
}

func describeEnsureLockfile(lockfiles pkgcontext.LockfileController, forceUpdate bool) {
	if forceUpdate {
		lockfiles.ForceUpdateLockfile()
		print.Verb("lockfile cleared, resolving fresh dependency versions")
	}

	if lockfiles.HasLockfile() {
		print.Verb("using lockfile for reproducible dependency resolution")
		return
	}

	print.Verb("no lockfile found, will create one after ensuring dependencies")
}

func requireLockfileSupport(lockfiles pkgcontext.LockfileController) error {
	if lockfiles.HasLockfileResolver() {
		return nil
	}
	return errors.New("cannot use --lock-only without lockfile support")
}

func saveCommandLockfile(lockfiles pkgcontext.LockfileController) error {
	return lockfiles.SaveLockfile()
}

func lockfileDependencyCount(lockfiles pkgcontext.LockfileController) int {
	lf := lockfiles.GetLockfile()
	if lf == nil {
		return 0
	}
	return lf.DependencyCount()
}

func persistBuildLockfile(
	lockfiles pkgcontext.BuildLockfileController,
	compilerVersion string,
	compilerPreset string,
	entry string,
	output string,
) error {
	lockfiles.RecordBuildToLockfile(compilerVersion, compilerPreset, entry, output)
	return lockfiles.SaveLockfile()
}
