package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

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

	// Initialize lockfile resolver if lockfile support is enabled
	if useLockfile {
		if err = initLockfileResolver(c, pcx); err != nil {
			return errors.Wrap(err, "failed to initialize lockfile resolver")
		}

		describeEnsureLockfile(pcx, forceUpdate)
	}

	// If lock-only mode, just save the lockfile without ensuring dependencies
	if lockOnly {
		if err := requireLockfileSupport(pcx); err != nil {
			return err
		}
		err = saveCommandLockfile(pcx)
		if err != nil {
			return errors.Wrap(err, "failed to save lockfile")
		}
		print.Verb("lockfile updated")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	updated, err := pcx.EnsureProject(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies")
	}
	if updated {
		print.Verb("updated package dependencies with latest tags")
	}

	if useLockfile {
		count := lockfileDependencyCount(pcx)
		if count > 0 {
			print.Verb("lockfile saved with", count, "dependencies")
		}
	}

	print.Verb("ensured dependencies for package")

	return nil
}
