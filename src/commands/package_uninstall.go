package commands

import (
	"slices"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func packageUninstallFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "project directory",
		},
		cli.BoolFlag{
			Name:  "dev",
			Usage: "remove development dependencies",
		},
	}
}

//nolint:dupl
func packageUninstall(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))
	development := c.Bool("dev")

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "uninstall", 0)
		return nil
	}

	deps := []versioning.DependencyString{}
	for _, dep := range c.Args() {
		deps = append(deps, versioning.DependencyString(dep))
	}

	pcx, _, err := loadDependencyOperationContext(c, dir)
	if err != nil {
		return reportDependencyCommandError(c, "uninstall", errors.Wrap(err, "failed to load the Pawn package"))
	}

	previous := slices.Clone(pcx.Package.GetAllDependencies())
	err = pcx.Uninstall(deps, development)
	if err != nil {
		return reportDependencyCommandError(c, "uninstall", err)
	}

	if slices.Equal(previous, pcx.Package.GetAllDependencies()) {
		print.Info("Dependency list unchanged")
	} else {
		print.Info("Dependency list updated")
	}

	return nil
}
