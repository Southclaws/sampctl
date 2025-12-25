package commands

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

type commandEnv struct {
	CacheDir string
	Platform string
	Verbose  bool
}

func applyVerboseFlag(c *cli.Context) bool {
	verbose := c.GlobalBool("verbose") || c.Bool("verbose")
	if verbose {
		print.SetVerbose()
	}
	return verbose
}

func getCommandEnv(c *cli.Context) (commandEnv, error) {
	verbose := applyVerboseFlag(c)
	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return commandEnv{}, errors.Wrap(err, "failed to get config dir")
	}
	return commandEnv{CacheDir: cacheDir, Platform: platform(c), Verbose: verbose}, nil
}
