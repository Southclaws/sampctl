package main

import (
	"context"
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
		Usage: "build configuration to use",
	},
	cli.BoolFlag{
		Name:  "forceEnsure",
		Usage: "forces dependency ensure before build",
	},
	cli.BoolFlag{
		Name:  "dryRun",
		Usage: "does not run the build but outputs the command necessary to do so",
	},
	cli.BoolFlag{
		Name:  "watch",
		Usage: "keeps sampctl running and triggers builds whenever source files change",
	},
	cli.StringFlag{
		Name:  "buildFile",
		Value: "",
		Usage: "declares a file to store the incrementing build number for easy versioning",
	},
	cli.BoolFlag{
		Name:  "relativePaths",
		Usage: "force compiler output to use relative paths instead of absolute",
	},
}

func packageBuild(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))
	forceEnsure := c.Bool("forceEnsure")
	dryRun := c.Bool("dryRun")
	watch := c.Bool("watch")
	buildFile := c.String("buildFile")
	relativePaths := c.Bool("relativePaths")

	build := c.Args().Get(1)

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return errors.Wrap(err, "failed to get or create cache directory")
	}

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	if watch {
		err := rook.BuildWatch(context.Background(), gh, gitAuth, &pkg, build, cacheDir, appRuntime.GOOS, forceEnsure, buildFile, relativePaths, nil)
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}
	} else {
		problems, result, err := rook.Build(context.Background(), gh, gitAuth, &pkg, build, cacheDir, appRuntime.GOOS, forceEnsure, dryRun, relativePaths, buildFile)
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}

		if len(problems.Errors()) > 0 {
			print.Erro("Build failed with", len(problems), "problems")
		} else if len(problems.Warnings()) > 0 {
			print.Warn("Build complete with", len(problems), "problems")
		} else {
			print.Info("Build successful with", len(problems), "problems")
		}

		print.Verb(fmt.Sprintf("Results, in bytes: Header: %d, Code: %d, Data: %d, Stack/Heap: %d, Estimated usage: %d, Total: %d\n",
			result.Header,
			result.Code,
			result.Data,
			result.StackHeap,
			result.Estimate,
			result.Total))
	}

	return nil
}

func packageBuildBash(c *cli.Context) {
	dir := util.FullPath(c.String("dir"))

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return
	}

	for _, b := range pkg.Builds {
		fmt.Println(b.Name)
	}
}
