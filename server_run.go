package main

import (
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var serverRunFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "container",
		Usage: "starts the server as a Linux container instead of running it in the current directory",
	},
}

func serverRun(c *cli.Context) error {
	dir := util.FullPath(c.String("dir"))
	container := c.Bool("container")

	cfg, err := runtime.NewConfigFromEnvironment(dir)
	if err != nil {
		return nil
	}

	err = runtime.Ensure(&cfg)
	if err != nil {
		return err
	}

	if container {
		err = runtime.RunContainer(cfg, c.App.Version)
	} else {
		err = runtime.Run(cfg)
	}

	return err
}
