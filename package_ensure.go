package main

import (
	"context"
	"runtime"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageEnsureFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageEnsure(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	dir := util.FullPath(c.String("dir"))

	pkg, err := rook.PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	err = rook.EnsureDependencies(ctx, gh, &pkg, gitAuth, runtime.GOOS, cacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to ensure")
	}

	print.Info("ensured dependencies for package")

	return nil
}
