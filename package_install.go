package main

import (
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

var packageInstallFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageInstall(c *cli.Context) error {
	dir := util.FullPath(c.String("dir"))

	dep := versioning.DependencyString(c.Args().First())
	if dep == "" {
		cli.ShowCommandHelpAndExit(c, "install", 0)
		return nil
	}

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = pkg.Install(dep)
	if err != nil {
		return err
	}

	fmt.Println("successfully ensured dependencies for project")

	return nil
}
