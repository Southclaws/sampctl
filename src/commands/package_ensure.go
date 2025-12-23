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
		Usage: "update cached dependencies to latest version",
	},
}

func packageEnsure(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config dir")
	}
	dir := fs.MustAbs(c.String("dir"))
	forceUpdate := c.Bool("update")

	pcx, err := pkgcontext.NewPackageContext(
		gh, nil, true, dir, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to create package context")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	err = pcx.EnsureDependencies(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies")
	}

	print.Info("ensured dependencies for package")

	return nil
}
