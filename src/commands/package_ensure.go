package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
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
		gh, nil, true, dir, env.Platform, env.CacheDir, "", false)
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
		if forceUpdate && pcx.LockfileResolver != nil {
			pcx.LockfileResolver.ForceUpdate()
			print.Info("lockfile cleared, resolving fresh dependency versions")
		}

		// Report lockfile status
		if pcx.HasLockfile() {
			print.Info("using lockfile for reproducible dependency resolution")
		} else {
			print.Info("no lockfile found, will create one after ensuring dependencies")
		}
	}

	// If lock-only mode, just save the lockfile without ensuring dependencies
	if lockOnly {
		if pcx.LockfileResolver == nil {
			return errors.New("cannot use --lock-only without lockfile support")
		}
		err = pcx.SaveLockfile()
		if err != nil {
			return errors.Wrap(err, "failed to save lockfile")
		}
		print.Info("lockfile updated")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	updated, err := pcx.TagTaglessDependencies(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to tag tagless dependencies")
	}
	if updated {
		print.Info("updated package dependencies with latest tags")
	}

	err = pcx.EnsureDependencies(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies")
	}

	// Save lockfile after successful ensure
	if useLockfile {
		err = pcx.SaveLockfile()
		if err != nil {
			print.Warn("failed to save lockfile:", err)
		} else if pcx.LockfileResolver != nil {
			lf := pcx.LockfileResolver.GetLockfile()
			if lf != nil {
				print.Info("lockfile saved with", lf.DependencyCount(), "dependencies")
			}
		}
	}

	print.Info("ensured dependencies for package")

	return nil
}

// packageLockfileStatus displays lockfile status for a package
func packageLockfileStatus(dir string) error {
	if !lockfile.Exists(dir) {
		print.Info("no lockfile found in", dir)
		return nil
	}

	lf, err := lockfile.Load(dir)
	if err != nil {
		return errors.Wrap(err, "failed to load lockfile")
	}

	print.Info("lockfile found:", lockfile.GetPath(dir))
	print.Info("  version:", lf.Version)
	print.Info("  generated:", lf.Generated.Format(time.RFC3339))
	print.Info("  sampctl version:", lf.SampctlVersion)
	print.Info("  total dependencies:", lf.DependencyCount())
	print.Info("  direct dependencies:", len(lf.DirectDependencies()))
	print.Info("  transitive dependencies:", len(lf.TransitiveDependencies()))

	return nil
}
