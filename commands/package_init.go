package commands

import (
	"context"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

var PackageInitFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
}

func PackageInit(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	config, err := types.LoadOrCreateConfig(cacheDir)
	if err != nil {
		return errors.Wrapf(err, "Failed to load or create sampctl config in %s", cacheDir)
	}

	_, err = rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
	if err == nil {
		return errors.New("Directory already appears to be a package")
	}

	err = rook.Init(context.Background(), gh, dir, config, gitAuth, platform(c), cacheDir)

	return err
}
