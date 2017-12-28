package main

import (
	"fmt"
	appRuntime "runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
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
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))
	build := c.String("build")
	forceEnsure := c.Bool("forceEnsure")

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return errors.Wrap(err, "failed to get or create cache directory")
	}

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	problems, result, err := rook.Build(&pkg, build, cacheDir, appRuntime.GOOS, forceEnsure)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	print.Info("Build complete with", len(problems), "problems")
	print.Info(fmt.Sprintf("Results, in bytes: Header: %d, Code: %d, Data: %d, Stack/Heap: %d, Estimated usage: %d, Total: %d\n",
		result.Header,
		result.Code,
		result.Data,
		result.StackHeap,
		result.Estimate,
		result.Total))

	return nil
}
