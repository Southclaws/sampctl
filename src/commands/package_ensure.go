package commands

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
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

	runtimeName := c.Args().Get(0)

	cacheDir := download.GetCacheDir()

	dir := util.FullPath(c.String("dir"))
	forceUpdate := c.Bool("update")

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	pcx.ActualRuntime, err = pcx.Package.GetRuntimeConfig(runtimeName)
	if err != nil {
		return err
	}
	pcx.ActualRuntime.Platform = pcx.Platform

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	err = pcx.EnsureDependencies(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure")
	}

	print.Info("ensured dependencies for package")

	return nil
}
