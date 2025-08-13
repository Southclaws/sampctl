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

var packageInitFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func packageInit(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))

	cacheDir := download.GetCacheDir()

	_, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "", true)
	if err != nil {
		return errors.New("Directory already appears to be a package")
	}

	err = rook.Init(context.Background(), gh, dir, cfg, gitAuth, platform(c), cacheDir)

	return err
}
