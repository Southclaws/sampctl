package main

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageRunFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "container",
		Usage: "starts the server as a Linux container instead of running it in the current directory",
	},
	cli.StringFlag{
		Name:  "build",
		Value: "",
		Usage: "build configuration to use if `--forceBuild` is set",
	},
	cli.BoolFlag{
		Name:  "forceBuild",
		Usage: "forces a build to run before executing the server",
	},
	cli.BoolFlag{
		Name:  "forceEnsure",
		Usage: "forces dependency ensure before build if `--forceBuild` is set",
	},
	cli.BoolFlag{
		Name:  "noCache",
		Usage: "forces download of plugins if `--forceEnsure` is set",
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

func packageRun(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))
	container := c.Bool("container")
	build := c.String("build")
	forceBuild := c.Bool("forceBuild")
	forceEnsure := c.Bool("forceEnsure")
	noCache := c.Bool("noCache")
	watch := c.Bool("watch")
	buildFile := c.String("buildFile")
	relativePaths := c.Bool("relativePaths")

	runtimeName := c.Args().Get(0)

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package run",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("container", container).
				Set("build", build).
				Set("forceBuild", forceBuild).
				Set("forceEnsure", forceEnsure).
				Set("noCache", noCache).
				Set("watch", watch).
				Set("buildFile", buildFile != "").
				Set("relativePaths", relativePaths),
		})
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	pcx.Runtime = runtimeName
	pcx.Container = container
	pcx.AppVersion = c.App.Version
	pcx.CacheDir = cacheDir
	pcx.BuildName = build
	pcx.ForceBuild = forceBuild
	pcx.ForceEnsure = forceEnsure
	pcx.NoCache = noCache
	pcx.BuildFile = buildFile
	pcx.Relative = relativePaths

	if watch {
		err = pcx.RunWatch(context.Background())
	} else {
		err = pcx.Run(context.Background(), os.Stdout, os.Stdin)
	}
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	return nil
}
