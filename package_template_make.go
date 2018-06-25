package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
)

var packageTemplateMakeFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the package - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "update",
		Usage: "update cached dependencies to latest version",
	},
}

func packageTemplateMake(c *cli.Context) (err error) {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	if config.Metrics {
		segment.Enqueue(analytics.Track{
			Event:  "package template make",
			UserId: config.UserID,
		})
	}

	dir := util.FullPath(c.String("dir"))
	forceUpdate := c.Bool("update")

	if len(c.Args()) != 1 {
		cli.ShowCommandHelpAndExit(c, "make", 0)
		return nil
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}
	name := c.Args().First()

	pcx, err := rook.NewPackageContext(gh, gitAuth, true, dir, platform(c), cacheDir, "")
	if err != nil {
		return
	}

	templatePath := filepath.Join(cacheDir, "templates", name)
	if util.Exists(templatePath) {
		return errors.New("A template with that name already exists")
	}

	err = os.MkdirAll(templatePath, 0700)
	if err != nil {
		return errors.Wrapf(err, "failed to create template directory at '%s'", templatePath)
	}

	pcx.Package.LocalPath = templatePath
	pcx.Package.Vendor = filepath.Join(templatePath, "dependencies")
	pcx.Package.Repo = name
	pcx.Package.Entry = "tmpl.pwn"
	pcx.Package.Output = "tmpl.amx"

	err = pcx.Package.WriteDefinition()
	if err != nil {
		return errors.Wrap(err, "failed to write package template definition file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	err = pcx.EnsureDependencies(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies of template package")
	}

	print.Info("Template successfully created at", templatePath)
	print.Info("To modify this template, such as install dependencies, either:")
	print.Info(fmt.Sprintf("- `cd %s` and use sampctl as normal", templatePath))
	print.Info(fmt.Sprintf("- pass `--dir %s` to sampctl commands", templatePath))

	return
}
