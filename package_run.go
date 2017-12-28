package main

import (
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
		Name:  "version",
		Value: "0.3.7",
		Usage: "the SA:MP server version to use",
	},
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
	cli.StringFlag{
		Name:  "endpoint",
		Value: "http://files.sa-mp.com",
		Usage: "endpoint to download packages from",
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
}

func packageRun(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	version := c.String("version")
	dir := util.FullPath(c.String("dir"))
	endpoint := c.String("endpoint")
	container := c.Bool("container")
	mountCache := c.Bool("mountCache")
	build := c.String("build")
	forceBuild := c.Bool("forceBuild")
	forceEnsure := c.Bool("forceEnsure")
	noCache := c.Bool("noCache")

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	cfg := types.Runtime{
		Platform:   runtime.GOOS,
		AppVersion: c.App.Version,
		Version:    version,
		Endpoint:   endpoint,
	}

	if container {
		cfg.Container = &types.ContainerConfig{MountCache: mountCache}
	}

	err = rook.Run(pkg, cfg, cacheDir, build, forceBuild, forceEnsure, noCache)

	return err
}
