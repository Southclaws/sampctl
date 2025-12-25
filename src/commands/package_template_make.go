package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
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
	forceUpdate := c.Bool("update")

	if len(c.Args()) != 1 {
		cli.ShowCommandHelpAndExit(c, "make", 0)
		return nil
	}

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}
	name := c.Args().First()

	templatePath := filepath.Join(env.CacheDir, "templates", name)
	if fs.Exists(templatePath) {
		return errors.New("A template with that name already exists")
	}

	err = fs.EnsureDir(templatePath, fs.PermDirPrivate)
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
		gh, nil, true, templatePath, env.Platform, env.CacheDir, "", false)
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
