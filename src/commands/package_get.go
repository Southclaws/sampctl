package commands

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/rook"
	"gopkg.in/urfave/cli.v1"
)

var packageGetFlags = []cli.Flag{}

func packageGet(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "get", 0)
		return nil
	}

	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return err
	}

	dep, err := versioning.DependencyString(c.Args().First()).Explode()
	if err != nil {
		return err
	}

	dir := c.Args().Get(1)
	if dir == "" {
		dir = fs.MustAbs(".")
	}

	err = rook.Get(context.Background(), gh, dep, dir, gitAuth, platform(c), cacheDir)
	if err != nil {
		return err
	}

	print.Info("successfully cloned package")

	return nil
}

func packageGetBash(c *cli.Context) {
	completePackageList(c)
}
