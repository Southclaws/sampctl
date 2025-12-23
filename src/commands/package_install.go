package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

var packageInstallFlags = []cli.Flag{
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

//nolint:dupl
func packageInstall(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := fs.MustAbs(c.String("dir"))
	development := c.Bool("dev")

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "install", 0)
		return nil
	}

	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config dir")
	}

	deps := []versioning.DependencyString{}
	for _, dep := range c.Args() {
		deps = append(deps, versioning.DependencyString(dep))
	}

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	err = pcx.Install(context.Background(), deps, development)
	if err != nil {
		return err
	}

	print.Info("successfully added new dependency")

	return nil
}

func packageInstallBash(c *cli.Context) {
	cacheDir, err := fs.ConfigDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to get config dir:", err)
		return
	}

	packages, err := download.GetPackageList(cacheDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to get package list:", err)
		return
	}

	query := c.Args().First()
	for _, pkg := range packages {
		if strings.HasPrefix(pkg.String(), query) {
			fmt.Println(pkg)
		}
	}
}
