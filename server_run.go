package main

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

var serverRunFlags = []cli.Flag{
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
	cli.BoolFlag{
		Name:  "forceEnsure",
		Usage: "forces plugin and binaries ensure before run",
	},
	cli.BoolFlag{
		Name:  "noCache",
		Usage: "forces download of plugins if `--forceEnsure` is set",
	},
}

func serverRun(c *cli.Context) error {
	dir := util.FullPath(c.String("dir"))
	container := c.Bool("container")
	mountCache := c.Bool("mountCache")
	forceEnsure := c.Bool("forceEnsure")
	noCache := c.Bool("noCache")

	cfg, err := runtime.NewConfigFromEnvironment(dir)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as server runtime environment")
	}

	if forceEnsure {
		err = runtime.Ensure(&cfg, noCache)
		if err != nil {
			return err
		}
	}

	if container {
		cfg.Container = &types.ContainerConfig{MountCache: mountCache}
		cfg.AppVersion = c.App.Version
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return errors.Wrap(err, "failed to get or create cache directory")
	}

	err = runtime.Run(cfg, cacheDir)

	return err
}
