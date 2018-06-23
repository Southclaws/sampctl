package main

import (
	"context"
	"runtime"
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
}

func packageEnsure(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	runtimeName := c.Args().Get(0)
	if runtimeName == "" {
		runtimeName = "default"
	}

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package run",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("runtime", runtimeName != "default"),
		})
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	dir := util.FullPath(c.String("dir"))

	pkg, err := rook.PackageFromDir(true, dir, runtime.GOOS, cacheDir, "", gitAuth)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	pkg.Runtime = rook.GetRuntimeConfig(pkg, runtimeName)

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	err = rook.EnsureDependencies(ctx, gh, &pkg, gitAuth, runtime.GOOS, cacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to ensure")
	}

	print.Info("ensured dependencies for package")

	return nil
}
