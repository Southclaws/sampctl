package commands

import (
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
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

	print.Warn("The use of 'sampctl server' has been deprecated.")
	print.Warn("Please instead use 'sampctl package init' to generate a new config file.")
	print.Warn("You can learn more here: https://github.com/Southclaws/sampctl/wiki")

	return nil
}
