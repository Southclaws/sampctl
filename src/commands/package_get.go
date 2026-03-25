package commands

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/rook"
	"gopkg.in/urfave/cli.v1"
)

func packageGetFlags() []cli.Flag {
	return nil
}

func packageGet(c *cli.Context) error {
	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "get", 0)
		return nil
	}

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}
	state, err := getCommandState(c)
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

	err = rook.Get(rook.GetOptions{
		Context:  context.Background(),
		GitHub:   state.gh,
		Meta:     dep,
		Dir:      dir,
		Auth:     state.gitAuth,
		Platform: env.Platform,
		CacheDir: env.CacheDir,
	})
	if err != nil {
		return err
	}

	print.Info("successfully cloned package")

	return nil
}

func packageGetBash(c *cli.Context) {
	completePackageList(c)
}
