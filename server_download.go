package main

import (
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/server"
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
	cli.StringFlag{
		Name:  "endpoint",
		Value: "http://files.sa-mp.com",
		Usage: "endpoint to download packages from",
	},
}

func serverDownload(c *cli.Context) error {
	version := c.String("version")
	dir := util.FullPath(c.String("dir"))
	endpoint := c.String("endpoint")
	return server.GetServerPackage(endpoint, version, dir)
}
