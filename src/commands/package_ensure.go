package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/sys/gitcheck"
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
			Usage: "project directory",
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
			Usage: "resolve dependencies without reading or writing pawn.lock",
		},
		cli.BoolFlag{
			Name:  "lock-only",
			Usage: "update pawn.lock without changing installed dependencies",
		},
	}
}

func packageEnsure(c *cli.Context) error {
	if err := gitcheck.RequireInstalled(); err != nil {
		return err
	}

	dir := fs.MustAbs(c.String("dir"))
	updateRequest, err := parseEnsureUpdateRequest(c)
	if err != nil {
		return &commandExitError{code: 1, err: err}
	}
	noLock := c.Bool("no-lock")
	lockOnly := c.Bool("lock-only")
	useLockfile := !noLock

	// Create package context
	pcx, _, err := loadDependencyOperationContext(c, dir)
	if err != nil {
		return reportDependencyCommandError(c, "ensure", errors.Wrap(err, "failed to load the Pawn package"))
	}

	state, err := getCommandState(c)
	if err != nil {
		return err
	}

	ctx, cancel := newCommandTimeoutContext(time.Hour)
	defer cancel()

	err = runPackageEnsure(ctx, pcx, ensureCommandOptions{
		version:     state.version,
		useLockfile: useLockfile,
		lockOnly:    lockOnly,
		update:      updateRequest,
	})
	return reportDependencyCommandError(c, "ensure", err)
}

func parseEnsureUpdateRequest(c *cli.Context) (pkgcontext.DependencyUpdateRequest, error) {
	if c.Bool("no-lock") && c.Bool("lock-only") {
		return pkgcontext.DependencyUpdateRequest{}, errors.New("cannot use --lock-only with --no-lock")
	}
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
			return pkgcontext.DependencyUpdateRequest{}, errors.Wrap(err, "invalid dependency argument")
		}

		request.Target = target
		request.TargetMeta = targetMeta
	}

	return request, nil
}

func runPackageEnsure(ctx context.Context, target ensureCommandTarget, opts ensureCommandOptions) error {
	if opts.useLockfile {
		if err := target.InitLockfileResolver(opts.version); err != nil {
			return &pkgcontext.OperationError{Operation: "read lockfile", Err: errors.Wrap(err, "failed to load the lockfile")}
		}

		describeEnsureLockfile(target, opts.update.Force && !opts.update.HasTarget())
	}

	if opts.lockOnly {
		if err := requireLockfileSupport(target); err != nil {
			return err
		}
		if err := updateCommandLockfile(ctx, target, opts.update); err != nil {
			return err
		}
		print.Info("Lockfile updated")
		return nil
	}

	updated, err := target.EnsureProject(ctx, opts.update)
	if err != nil {
		return errors.Wrap(err, "failed to install dependencies")
	}
	if updated {
		print.Verb("Updated dependencies to the latest tags")
	}

	if opts.useLockfile {
		count := lockfileDependencyCount(target)
		if count > 0 {
			print.Verb("lockfile saved with", count, "dependencies")
		}
	}

	print.Info("Dependencies and runtime are ready")

	return nil
}

func updateCommandLockfile(ctx context.Context, target ensureCommandTarget, request pkgcontext.DependencyUpdateRequest) error {
	if transactional, ok := target.(interface {
		UpdateProjectLockfile(context.Context, pkgcontext.DependencyUpdateRequest) error
	}); ok {
		if err := transactional.UpdateProjectLockfile(ctx, request); err != nil {
			return errors.Wrap(err, "failed to update lockfile")
		}
		return nil
	}
	if err := target.UpdateLockfile(ctx, request); err != nil {
		return errors.Wrap(err, "failed to update lockfile")
	}
	if err := saveCommandLockfile(target); err != nil {
		return &pkgcontext.OperationError{Operation: "lockfile", Err: errors.Wrap(err, "failed to save lockfile")}
	}
	return nil
}
