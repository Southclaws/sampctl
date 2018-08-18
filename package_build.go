package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
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

	build := c.Args().Get(0)

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package build",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("forceEnsure", forceEnsure).
				Set("watch", watch).
				Set("watch", watch).
				Set("buildFile", buildFile != "").
				Set("build", build != ""),
		})
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return errors.Wrap(err, "failed to get or create cache directory")
	}

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	if watch {
		err := pcx.BuildWatch(context.Background(), build, forceEnsure, buildFile, relativePaths, nil)
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}
	} else {
		problems, result, err := pcx.Build(context.Background(), build, forceEnsure, dryRun, relativePaths, buildFile)
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}

		if build == "" {
			build = "default"
		}

		if problems.Fatal() {
			return cli.NewExitError(errors.New("Build encountered fatal error"), 1)
		} else if len(problems.Errors()) > 0 {
			return cli.NewExitError(errors.Errorf("Build failed with %d problems", len(problems)), 1)
		} else if len(problems.Warnings()) > 0 {
			print.Warn("Build", build, "complete with", len(problems), "problems")
		} else {
			print.Info("Build", build, "successful with", len(problems), "problems")
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

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, runtime.GOOS, cacheDir, "")
	if err != nil {
		return
	}

	for _, b := range pcx.Package.Builds {
		fmt.Println(b.Name)
	}
}
