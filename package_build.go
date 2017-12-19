package main

import (
	"fmt"
	appRuntime "runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageBuildFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
	cli.StringFlag{
		Name:  "build",
		Value: "",
		Usage: "build configuration to use if `--forceBuild` is set",
	},
	cli.BoolFlag{
		Name:  "forceEnsure",
		Usage: "forces dependency ensure before build if `--forceBuild` is set",
	},
}

func packageBuild(c *cli.Context) error {
	dir := util.FullPath(c.String("dir"))
	build := c.String("build")
	forceEnsure := c.Bool("forceEnsure")

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	output, err := rook.Build(&pkg, build, appRuntime.GOOS, forceEnsure)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	fmt.Println("successfully built project to", output)

	return nil
}
