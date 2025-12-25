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
		Name:  "preset",
		Value: "samp",
		Usage: "specify the preset to use for the project: 'samp' for SA-MP or 'openmp' for open.mp",
	},
}

func packageInit(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))
	preset := c.String("preset")

	if preset != "samp" && preset != "openmp" {
		print.Warn("Invalid preset option provided, defaulting to 'samp'.")
		preset = "samp"
	}

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}

	_, err = pkgcontext.NewPackageContext(gh, gitAuth, true, dir, env.Platform, env.CacheDir, "", true)
	if err != nil {
		return errors.New("Directory already appears to be a package")
	}

	err = rook.Init(context.Background(), gh, dir, cfg, gitAuth, env.Platform, env.CacheDir, preset)

	return err
}
