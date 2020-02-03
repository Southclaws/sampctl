package commands

import (
	appRuntime "runtime"

	"github.com/urfave/cli/v2"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var ServerDownloadFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "version",
		Value: "0.3.7",
		Usage: "the SA:MP server version to use",
	},
	&cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
}

func ServerDownload(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	version := c.String("version")
	dir := util.FullPath(c.String("dir"))

	return runtime.GetServerPackage(version, dir, appRuntime.GOOS)
}
