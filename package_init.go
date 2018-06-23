package main

import (
	"context"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageInitFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageInit(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package init",
			UserId: config.UserID,
		})
	}

	dir := util.FullPath(c.String("dir"))

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	_, err = rook.PackageFromDir(true, dir, runtime.GOOS, cacheDir, "", gitAuth)
	if err == nil {
		return errors.New("Directory already appears to be a package")
	}

	err = rook.Init(context.Background(), gh, dir, config, gitAuth, runtime.GOOS, cacheDir)

	return err
}
