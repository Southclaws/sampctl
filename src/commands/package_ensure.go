package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

type ensureCommandTarget interface {
	pkgcontext.LockfileInitializer
	pkgcontext.LockfileController
	pkgcontext.LockfileUpdater
	EnsureProject(ctx context.Context, request pkgcontext.DependencyUpdateRequest) (bool, error)
}

type ensureCommandOptions struct {
	version     string
	useLockfile bool
	lockOnly    bool
	update      pkgcontext.DependencyUpdateRequest
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
			Usage: "update dynamic dependencies (`user/repo` or `user/repo:latest`) to the latest tagged release",
		},
		cli.BoolFlag{
			Name:  "force",
			Usage: "with `--update`, also update dependencies pinned to explicit tags",
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
	updateRequest, err := parseEnsureUpdateRequest(c)
	if err != nil {
		return err
	}
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
		useLockfile: useLockfile,
		lockOnly:    lockOnly,
		update:      updateRequest,
	})
}

func parseEnsureUpdateRequest(c *cli.Context) (pkgcontext.DependencyUpdateRequest, error) {
	request := pkgcontext.DependencyUpdateRequest{
		Enabled: c.Bool("update"),
		Force:   c.Bool("force"),
	}

	if request.Force && !request.Enabled {
		return pkgcontext.DependencyUpdateRequest{}, errors.New("cannot use --force without --update")
	}

	if len(c.Args()) > 1 {
		return pkgcontext.DependencyUpdateRequest{}, errors.New("ensure accepts at most one dependency argument")
	}

	if len(c.Args()) == 1 {
		if !request.Enabled {
			return pkgcontext.DependencyUpdateRequest{}, errors.New("dependency arguments require --update")
		}

		target := c.Args().First()
		targetMeta, err := versioning.DependencyString(target).Explode()
		if err != nil {
			return pkgcontext.DependencyUpdateRequest{}, errors.Wrap(err, "failed to parse dependency selector")
		}

		request.Target = target
		request.TargetMeta = targetMeta
	}

	return request, nil
}

func runPackageEnsure(ctx context.Context, target ensureCommandTarget, opts ensureCommandOptions) error {
	if opts.useLockfile {
		if err := target.InitLockfileResolver(opts.version); err != nil {
			return errors.Wrap(err, "failed to initialize lockfile resolver")
		}

		describeEnsureLockfile(target, opts.update.Force && !opts.update.HasTarget())
	}

	if opts.lockOnly {
		if err := requireLockfileSupport(target); err != nil {
			return err
		}
		if err := target.UpdateLockfile(ctx, opts.update); err != nil {
			return errors.Wrap(err, "failed to update lockfile")
		}
		if err := saveCommandLockfile(target); err != nil {
			return errors.Wrap(err, "failed to save lockfile")
		}
		print.Verb("lockfile updated")
		return nil
	}

	updated, err := target.EnsureProject(ctx, opts.update)
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
