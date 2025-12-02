package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
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

	forceUpdate := c.Bool("update")

	if len(c.Args()) != 1 {
		cli.ShowCommandHelpAndExit(c, "make", 0)
		return nil
	}

	cacheDir := util.GetConfigDir()
	name := c.Args().First()

	templatePath := filepath.Join(cacheDir, "templates", name)
	if util.Exists(templatePath) {
		return errors.New("A template with that name already exists")
	}

	err = os.MkdirAll(templatePath, 0700)
	if err != nil {
		return errors.Wrapf(err, "failed to create template directory at '%s'", templatePath)
	}

	pkg := pawnpackage.Package{
		LocalPath: templatePath,
		Entry:     "tmpl.pwn",
		Output:    "tmpl.amx",
	}
	pkg.Repo = name

	err = pkg.WriteDefinition()
	if err != nil {
		return errors.Wrap(err, "failed to write package template definition file")
	}

	pcx, err := pkgcontext.NewPackageContext(
		gh, nil, true, templatePath, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to create package context")
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

	return nil
}
