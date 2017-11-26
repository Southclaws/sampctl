package main

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
	
	"github.com/Southclaws/sampctl/server"
	"github.com/Southclaws/sampctl/settings"
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

	err := settings.InitialiseServer(version, dir)
	if err != nil {
		return errors.Wrap(err, "failed to initialise server")
	}

	err = server.GetServerPackage(endpoint, version, dir)
	if err != nil {
		return errors.Wrap(err, "failed to get package")
	}

	return nil
}
