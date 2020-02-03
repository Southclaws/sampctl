package commands

import (
	"context"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var PackageReleaseFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func PackageRelease(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = rook.Release(context.Background(), gh, gitAuth, pcx.Package)
	if err != nil {
		return errors.Wrap(err, "failed to release")
	}

	return nil
}
