package main

import (
	"context"
	"os"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/types"
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
	cli.BoolFlag{
		Name:  "mountCache",
		Usage: "if `--container` is set, mounts the local cache directory inside the container",
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
	mountCache := c.Bool("mountCache")
	build := c.String("build")
	forceBuild := c.Bool("forceBuild")
	forceEnsure := c.Bool("forceEnsure")
	noCache := c.Bool("noCache")
	watch := c.Bool("watch")
	buildFile := c.String("buildFile")
	relativePaths := c.Bool("relativePaths")

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	pkg, err := rook.PackageFromDir(true, dir, runtime.GOOS, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	runner := rook.Runner{
		Pkg: pkg,
		Config: types.Runtime{
			AppVersion: c.App.Version,
			Version:    pkg.Runtime.Version,
		},
		GitHub:      gh,
		Auth:        gitAuth,
		CacheDir:    cacheDir,
		Build:       build,
		ForceBuild:  forceBuild,
		ForceEnsure: forceEnsure,
		NoCache:     noCache,
		BuildFile:   buildFile,
		Relative:    relativePaths,
	}

	if container {
		runner.Config.Container = &types.ContainerConfig{MountCache: mountCache}
		runner.Config.Platform = "linux"
	} else {
		runner.Config.Platform = runtime.GOOS
	}

	if watch {
		err = runner.RunWatch(context.Background())
	} else {
		err = runner.Run(context.Background(), os.Stdout, os.Stdin)
	}
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	return nil
}
