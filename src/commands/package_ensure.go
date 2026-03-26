package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/sys/gitcheck"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

type ensureCommandTarget interface {
	pkgcontext.LockfileInitializer
	pkgcontext.LockfileController
	pkgcontext.LockfileUpdater
	EnsureProject(ctx context.Context, forceUpdate bool) (bool, error)
}

type ensureCommandOptions struct {
	version     string
	forceUpdate bool
	useLockfile bool
	lockOnly    bool
}

func packageEnsureFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "working directory for the project - by default, uses the current directory",
		},
		cli.BoolFlag{
			Name:  "update",
			Usage: "update cached dependencies to latest version, ignoring lockfile",
		},
		cli.BoolFlag{
			Name:  "no-lock",
			Usage: "disable lockfile support (not recommended for reproducible builds)",
		},
		cli.BoolFlag{
			Name:  "lock-only",
			Usage: "only update the lockfile without modifying dependencies",
		},
	}
}

func packageEnsure(c *cli.Context) error {
	if err := gitcheck.RequireInstalled(); err != nil {
		return err
	}

	dir := fs.MustAbs(c.String("dir"))
	forceUpdate := c.Bool("update")
	noLock := c.Bool("no-lock")
	lockOnly := c.Bool("lock-only")
	useLockfile := !noLock

	// Create package context
	pcx, _, err := loadPackageContext(c, dir, false)
	if err != nil {
		return errors.Wrap(err, "failed to create package context")
	}

	state, err := getCommandState(c)
	if err != nil {
		return err
	}

	ctx, cancel := newCommandTimeoutContext(time.Hour)
	defer cancel()

	return runPackageEnsure(ctx, pcx, ensureCommandOptions{
		version:     state.version,
		forceUpdate: forceUpdate,
		useLockfile: useLockfile,
		lockOnly:    lockOnly,
	})
}

func runPackageEnsure(ctx context.Context, target ensureCommandTarget, opts ensureCommandOptions) error {
	if opts.useLockfile {
		if err := target.InitLockfileResolver(opts.version); err != nil {
			return errors.Wrap(err, "failed to initialize lockfile resolver")
		}

		describeEnsureLockfile(target, opts.forceUpdate)
	}

	if opts.lockOnly {
		if err := requireLockfileSupport(target); err != nil {
			return err
		}
		if err := target.UpdateLockfile(ctx, opts.forceUpdate); err != nil {
			return errors.Wrap(err, "failed to update lockfile")
		}
		if err := saveCommandLockfile(target); err != nil {
			return errors.Wrap(err, "failed to save lockfile")
		}
		print.Verb("lockfile updated")
		return nil
	}

	updated, err := target.EnsureProject(ctx, opts.forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies")
	}
	if updated {
		print.Verb("updated package dependencies with latest tags")
	}

	if opts.useLockfile {
		count := lockfileDependencyCount(target)
		if count > 0 {
			print.Verb("lockfile saved with", count, "dependencies")
		}
	}

	print.Verb("ensured dependencies for package")

	return nil
}
