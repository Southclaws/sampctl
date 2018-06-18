package main

import (
	appRuntime "runtime"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var serverInitFlags = []cli.Flag{
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

func serverInit(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	version := c.String("version")
	dir := util.FullPath(c.String("dir"))

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "server init",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("version", version),
		})
	}

	err := runtime.InitialiseServer(version, dir, appRuntime.GOOS)
	if err != nil {
		return errors.Wrap(err, "failed to initialise server")
	}

	err = runtime.GetServerPackage(version, dir, appRuntime.GOOS)
	if err != nil {
		return errors.Wrap(err, "failed to get package")
	}

	return nil
}
