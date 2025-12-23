package commands

import (
	"context"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/package/rook"
)

var packageInitFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
	cli.StringFlag{
		Name:  "runtime",
		Value: "samp",
		Usage: "default target runtime for the project: 'samp' for SA-MP or 'openmp' for Open.MP",
	},
}

func packageInit(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := fs.MustAbs(c.String("dir"))
	runtime := c.String("runtime")

	if runtime != "samp" && runtime != "openmp" {
		print.Warn("Invalid runtime option provided, defaulting to 'samp'.")
		runtime = "samp"
	}

	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config dir")
	}

	_, err = pkgcontext.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "", true)
	if err != nil {
		return errors.New("Directory already appears to be a package")
	}

	err = rook.Init(context.Background(), gh, dir, cfg, gitAuth, platform(c), cacheDir, runtime)

	return err
}
