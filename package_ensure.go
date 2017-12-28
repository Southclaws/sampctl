package main

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageEnsureFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageEnsure(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = rook.EnsureDependencies(&pkg)
	if err != nil {
		return errors.Wrap(err, "failed to ensure")
	}

	print.Info("ensured dependencies for package")

	return nil
}
