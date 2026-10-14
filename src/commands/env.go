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
		return commandEnv{}, errors.Wrap(err, "failed to find the sampctl config directory")
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
	return loadPackageContextWithCache(c, dir, init, true, false)
}

func loadDependencyOperationContext(c *cli.Context, dir string) (*pkgcontext.PackageContext, commandEnv, error) {
	ctx, cancel := newCommandContext()
	defer cancel()

	if _, err := pkgcontext.RecoverDependencyOperation(ctx, dir); err != nil {
		return nil, commandEnv{}, &pkgcontext.OperationError{Operation: "recovery", Err: err}
	}

	return loadPackageContextWithCache(c, dir, false, false, false)
}

func loadPackageContextWithCache(c *cli.Context, dir string, init, cacheDependencies, useLockfile bool) (*pkgcontext.PackageContext, commandEnv, error) {
	env, err := getCommandEnv(c)
	if err != nil {
		return nil, commandEnv{}, err
	}

	state, err := getCommandState(c)
	if err != nil {
		return nil, commandEnv{}, err
	}

	pcx, err := pkgcontext.LoadPackageContext(pkgcontext.NewPackageContextOptions{
		GitHub:   state.gh,
		Auth:     state.gitAuth,
		Parent:   true,
		Dir:      dir,
		Platform: env.Platform,
		CacheDir: env.CacheDir,
		Init:     init,
	})
	if err != nil {
		if !cacheDependencies {
			return nil, commandEnv{}, &pkgcontext.OperationError{Operation: "read manifest", Err: err}
		}

		return nil, commandEnv{}, err
	}

	if useLockfile {
		if err := initLockfileResolver(c, pcx); err != nil {
			print.Warn("Failed to load the lockfile:", err)
		}
	}

	if cacheDependencies {
		print.Verb(pcx.Package, "building dependency tree and ensuring cached copies")
		if err := pcx.EnsureDependenciesCached(); err != nil {
			return nil, commandEnv{}, errors.Wrap(err, "failed to ensure dependencies are cached")
		}
		print.Verb(pcx.Package, "flattened dependencies to", len(pcx.AllDependencies), "leaves")
	}

	return pcx, env, nil
}
