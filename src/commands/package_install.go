package commands

import (
	"context"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

func packageInstallFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "working directory for the project - by default, uses the current directory",
		},
		cli.BoolFlag{
			Name:  "dev",
			Usage: "for specifying dependencies only necessary for development or testing of the package",
		},
	}
}

//nolint:dupl
func packageInstall(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))
	development := c.Bool("dev")

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "install", 0)
		return nil
	}

	deps := []versioning.DependencyString{}
	for _, dep := range c.Args() {
		deps = append(deps, versioning.DependencyString(dep))
	}

	pcx, _, err := loadPackageContext(c, dir, false)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	if err = initLockfileResolver(c, pcx); err != nil {
		return errors.Wrap(err, "failed to initialize lockfile resolver")
	}

	err = pcx.Install(context.Background(), deps, development)
	if err != nil {
		return err
	}

	//save lockfile after successful install
	err = pcx.SaveLockfile()
	if err != nil {
		print.Warn("failed to save lockfile:", err)
	}

	print.Info("successfully added new dependency")

	return nil
}

func packageInstallBash(c *cli.Context) {
	completePackageList(c)
}
