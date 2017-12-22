package main

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageInitFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageInit(c *cli.Context) error {
	dir := util.FullPath(c.String("dir"))

	_, err := rook.PackageFromDir(true, dir, "")
	if err == nil {
		return errors.New("Directory already appears to be a package")
	}

	err = rook.Init(dir)

	return err
}
