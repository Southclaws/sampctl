package commands

import (
	"context"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/runtime"
	"github.com/Southclaws/sampctl/util"
)

var serverEnsureFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the server - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "noCache",
		Usage: "forces download of plugins if `--forceEnsure` is set",
	},
}

func serverEnsure(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	print.Warn("The use of 'sampctl server' has been deprecated.")
	print.Warn("Follow this guide to upgrade: https://github.com/Southclaws/sampctl/wiki/samp.json-To-pawn.json")

	dir := util.FullPath(c.String("dir"))
	noCache := c.Bool("noCache")

	cfg, err := runtime.NewConfigFromEnvironment(dir)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = runtime.Ensure(context.Background(), gh, &cfg, noCache)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}
