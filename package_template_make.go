package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pkg/errors"
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
}

func packageTemplateMake(c *cli.Context) (err error) {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	dir := util.FullPath(c.String("dir"))

	if len(c.Args()) != 1 {
		cli.ShowCommandHelpAndExit(c, "make", 0)
		return nil
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}
	name := c.Args().First()

	pkg, err := rook.PackageFromDir(true, dir, runtime.GOOS, "")
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

	pkg.Local = templatePath
	pkg.Vendor = filepath.Join(templatePath, "dependencies")
	pkg.Repo = name
	pkg.Entry = "tmpl.pwn"
	pkg.Output = "tmpl.amx"

	err = pkg.WriteDefinition()
	if err != nil {
		return errors.Wrap(err, "failed to write package template definition file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	err = rook.EnsureDependencies(ctx, gh, &pkg, gitAuth, runtime.GOOS, cacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies of template package")
	}

	print.Info("Template successfully created at", templatePath)
	print.Info("To modify this template, such as install dependencies, either:")
	print.Info(fmt.Sprintf("- `cd %s` and use sampctl as normal", templatePath))
	print.Info(fmt.Sprintf("- pass `--dir %s` to sampctl commands", templatePath))

	return
}
