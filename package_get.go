package main

import (
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

var packageGetFlags = []cli.Flag{}

func packageGet(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "get", 0)
		return nil
	}

	dep, err := versioning.DependencyString(c.Args().First()).Explode()
	if err != nil {
		return err
	}

	dir := c.Args().Get(1)
	if dir == "" {
		dir = util.FullPath(".")
	}

	err = rook.Get(dep, dir)
	if err != nil {
		return err
	}

	print.Info("successfully cloned package")

	return nil
}
