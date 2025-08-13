package commands

import (
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

var packageUninstallFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "dev",
		Usage: "for specifying development dependencies",
	},
}

//nolint:dupl
func packageUninstall(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))
	development := c.Bool("dev")

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "uninstall", 0)
		return nil
	}

	cacheDir := download.GetCacheDir()

	deps := []versioning.DependencyString{}
	for _, dep := range c.Args() {
		deps = append(deps, versioning.DependencyString(dep))
	}

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = pcx.Uninstall(deps, development)
	if err != nil {
		return err
	}

	print.Info("successfully removed dependency")

	return nil
}
