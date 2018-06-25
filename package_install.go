package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
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

func packageInstall(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))
	development := c.Bool("dev")

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package install",
			UserId: config.UserID,
			Properties: analytics.NewProperties().
				Set("development", development),
		})
	}

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "install", 0)
		return nil
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ")
		return err
	}

	deps := []versioning.DependencyString{}
	for _, dep := range c.Args() {
		deps = append(deps, versioning.DependencyString(dep))
	}

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
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
	cacheDir, err := download.GetCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
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
