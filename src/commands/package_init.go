package commands

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/rook"
)

func packageInitFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "working directory for the project - by default, uses the current directory",
		},
		cli.StringFlag{
			Name:  "preset",
			Value: "openmp",
			Usage: "specify the preset to use for the project: 'samp' for SA-MP or 'openmp' for open.mp",
		},
	}
}

func packageInit(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))
	preset := c.String("preset")

	if preset != "samp" && preset != "openmp" {
		print.Warn("Invalid preset option provided, defaulting to 'openmp'.")
		preset = "openmp"
	}

	if err := validateInitDirectory(dir); err != nil {
		return err
	}

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}

	state, err := getCommandState(c)
	if err != nil {
		return err
	}
	cfg, err := getCommandConfig(c)
	if err != nil {
		return err
	}

	ctx, cancel := newCommandContext()
	defer cancel()

	err = rook.Init(rook.InitOptions{
		Context:  ctx,
		GitHub:   state.gh,
		Dir:      dir,
		Config:   cfg,
		Auth:     state.gitAuth,
		Platform: env.Platform,
		CacheDir: env.CacheDir,
		Preset:   preset,
		Version:  c.App.Version,
	})

	return err
}

func validateInitDirectory(dir string) error {
	pkg, err := pawnpackage.PackageFromDir(dir)
	if err != nil {
		return errors.Wrap(err, "failed to inspect package definition")
	}
	if pkg.Format != "" {
		return errors.New("Directory already appears to be a package")
	}
	return nil
}
