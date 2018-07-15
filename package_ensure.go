package main

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
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

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package run",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("runtime", runtimeName != ""),
		})
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	dir := util.FullPath(c.String("dir"))
	forceUpdate := c.Bool("update")

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	pcx.ActualRuntime, err = rook.GetRuntimeConfig(pcx.Package, runtimeName)
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
