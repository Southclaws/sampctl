package commands

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
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

func getCommandConfig(c *cli.Context) (*config.Config, error) {
	state, err := getCommandState(c)
	if err != nil {
		return nil, err
	}
	if state.cfg == nil {
		return nil, errors.New("config is not available")
	}
	return state.cfg, nil
}

func loadPackageContext(c *cli.Context, dir string, init bool) (*pkgcontext.PackageContext, commandEnv, error) {
	env, err := getCommandEnv(c)
	if err != nil {
		return nil, commandEnv{}, err
	}

	state, err := getCommandState(c)
	if err != nil {
		return nil, commandEnv{}, err
	}

	pcx, err := pkgcontext.NewPackageContext(pkgcontext.NewPackageContextOptions{
		GitHub:   state.gh,
		Auth:     state.gitAuth,
		Parent:   true,
		Dir:      dir,
		Platform: env.Platform,
		CacheDir: env.CacheDir,
		Init:     init,
	})
	if err != nil {
		return nil, commandEnv{}, err
	}

	return pcx, env, nil
}
