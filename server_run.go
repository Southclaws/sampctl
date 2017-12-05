package main

import (
	"fmt"

	"github.com/pkg/errors"
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

	var err error
	errs := runtime.ValidateServerDir(dir, version)
	if errs != nil {
		fmt.Println(errs)
		err = runtime.GetServerPackage(endpoint, version, dir)
		if err != nil {
			return errors.Wrap(err, "failed to get runtime package")
		}
	}

	if container {
		err = runtime.RunContainer(endpoint, version, dir, c.App.Version)
	} else {
		err = runtime.Run(endpoint, version, dir)
	}

	return err
}
