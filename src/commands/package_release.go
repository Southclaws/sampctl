package commands

import (
	"context"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/rook"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

var packageReleaseFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageRelease(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))

	cacheDir := download.GetCacheDir()

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = rook.Release(context.Background(), gh, gitAuth, pcx.Package)
	if err != nil {
		return errors.Wrap(err, "failed to release")
	}

	return nil
}
