package main

import (
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var serverRunFlags = []cli.Flag{
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
	cli.StringFlag{
		Name:  "endpoint",
		Value: "http://files.sa-mp.com",
		Usage: "endpoint to download packages from",
	},
	cli.BoolFlag{
		Name:  "container",
		Usage: "starts the server as a Linux container instead of running it in the current directory",
	},
}

func serverRun(c *cli.Context) error {
	version := c.String("version")
	dir := util.FullPath(c.String("dir"))
	endpoint := c.String("endpoint")
	container := c.Bool("container")

	cfg, err := runtime.NewConfigFromEnvironment(dir)
	if err != nil {
		return nil
	}

	cfg.Version = &version
	cfg.Endpoint = &endpoint

	if container {
		err = cfg.RunContainer(c.App.Version)
	} else {
		err = cfg.Run()
	}

	return err
}
