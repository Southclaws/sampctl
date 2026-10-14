package commands

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func packageTemplateMakeFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "package directory",
		},
		cli.BoolFlag{
			Name:  "update",
			Usage: "update cached dependencies to latest version",
		},
	}
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
		return errors.Errorf("template %q already exists", name)
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
		return errors.Wrap(err, "failed to save the template package definition")
	}

	pcx, _, err := loadPackageContext(c, templatePath, false)
	if err != nil {
		return errors.Wrap(err, "failed to load the template package")
	}

	ctx, cancel := newCommandTimeoutContext(time.Hour)
	defer cancel()

	err = pcx.EnsureDependencies(ctx, forceUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to install template dependencies")
	}

	print.Info("Template created at", templatePath)
	print.Info("To edit the template or install dependencies:")
	print.Info(fmt.Sprintf("- `cd %s` and run sampctl commands", templatePath))
	print.Info(fmt.Sprintf("- pass `--dir %s` to sampctl commands", templatePath))

	return nil
}
