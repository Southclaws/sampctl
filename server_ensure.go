package main

import (
	"context"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var serverEnsureFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "noCache",
		Usage: "forces download of plugins if `--forceEnsure` is set",
	},
}

func serverEnsure(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))
	noCache := c.Bool("noCache")

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "server ensure",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("noCache", noCache),
		})
	}

	cfg, err := runtime.NewConfigFromEnvironment(dir)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = runtime.Ensure(context.Background(), gh, &cfg, noCache)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}
