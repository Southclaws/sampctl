package main

import (
	appRuntime "runtime"

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
	return runtime.GetServerPackage(version, dir, appRuntime.GOOS)
}
