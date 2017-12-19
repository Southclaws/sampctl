package main

import (
	appRuntime "runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

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
	cli.StringFlag{
		Name:  "endpoint",
		Value: "http://files.sa-mp.com",
		Usage: "endpoint to download packages from",
	},
}

func serverInit(c *cli.Context) error {
	version := c.String("version")
	dir := util.FullPath(c.String("dir"))
	endpoint := c.String("endpoint")

	err := runtime.InitialiseServer(version, dir, appRuntime.GOOS)
	if err != nil {
		return errors.Wrap(err, "failed to initialise server")
	}

	err = runtime.GetServerPackage(endpoint, version, dir, appRuntime.GOOS)
	if err != nil {
		return errors.Wrap(err, "failed to get package")
	}

	return nil
}
