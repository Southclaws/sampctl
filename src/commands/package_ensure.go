package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

var packageEnsureFlags = []cli.Flag{
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

func packageEnsure(c *cli.Context) error {
	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}
	dir := fs.MustAbs(c.String("dir"))
	forceUpdate := c.Bool("update")
	noLock := c.Bool("no-lock")
	lockOnly := c.Bool("lock-only")
	useLockfile := !noLock

	// Create package context
	pcx, err := pkgcontext.NewPackageContext(
		gh, gitAuth, true, dir, env.Platform, env.CacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to create package context")
	}

	// Initialize lockfile resolver if lockfile support is enabled
	if useLockfile {
		err = pcx.InitLockfileResolver(sampctlVersion)
		if err != nil {
			return errors.Wrap(err, "failed to initialize lockfile resolver")
		}

		// If forcing update, clear the lockfile to resolve fresh versions
		if forceUpdate {
			pcx.ForceUpdateLockfile()
			print.Verb("lockfile cleared, resolving fresh dependency versions")
		}

		// Report lockfile status
		if pcx.HasLockfile() {
			print.Verb("using lockfile for reproducible dependency resolution")
		} else {
			print.Verb("no lockfile found, will create one after ensuring dependencies")
		}
	}

	// If lock-only mode, just save the lockfile without ensuring dependencies
	if lockOnly {
		if !pcx.HasLockfileResolver() {
			return errors.New("cannot use --lock-only without lockfile support")
		}
		err = pcx.SaveLockfile()
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
		lf := pcx.GetLockfile()
		if lf != nil {
			print.Verb("lockfile saved with", lf.DependencyCount(), "dependencies")
		}
	}

	print.Verb("ensured dependencies for package")

	return nil
}
