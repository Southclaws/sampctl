package main

import (
	appRuntime "runtime"

	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var serverDownloadFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "version",
		Value: "0.3.7",
		Usage: "the SA:MP server version to use",
	},
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
}

func serverDownload(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	version := c.String("version")
	dir := util.FullPath(c.String("dir"))

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "server download",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("version", version),
		})
	}

	return runtime.GetServerPackage(version, dir, appRuntime.GOOS)
}
