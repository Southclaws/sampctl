package main

import (
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/server"
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
}

func packageRun(c *cli.Context) error {
	version := c.String("version")
	projectDir := util.FullPath(c.String("dir"))
	endpoint := c.String("endpoint")
	container := c.Bool("container")
	build := c.String("build")
	forceBuild := c.Bool("forceBuild")
	forceEnsure := c.Bool("forceEnsure")

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		fmt.Println("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return err
	}

	pkg, err := rook.PackageFromDir(projectDir)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = server.PrepareRuntime(cacheDir, endpoint, version)
	if err != nil {
		return err
	}

	filename := util.FullPath(pkg.Output)
	if !util.Exists(filename) || forceBuild {
		filename, err = pkg.Build(build, forceEnsure)
		if err != nil {
			return err
		}
	}

	err = server.CopyFileToRuntime(cacheDir, version, filename)
	if err != nil {
		return err
	}

	runtimeDir := server.GetRuntimePath(cacheDir, version)

	if container {
		err = server.RunContainer(endpoint, version, runtimeDir, c.App.Version)
	} else {
		err = server.Run(endpoint, version, runtimeDir)
	}

	return err
}
