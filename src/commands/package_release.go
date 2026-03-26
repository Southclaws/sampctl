package commands

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/rook"
)

func packageReleaseFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "working directory for the project - by default, uses the current directory",
		},
	}
}

func packageRelease(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))

	pcx, _, err := loadPackageContext(c, dir, false)
	if err != nil {
		return err
	}
	state, err := getCommandState(c)
	if err != nil {
		return err
	}

	ctx, cancel := newCommandContext()
	defer cancel()

	err = rook.Release(ctx, state.gh, state.gitAuth, pcx.Package)
	if err != nil {
		return errors.Wrap(err, "failed to release")
	}

	return nil
}
