package commands

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

var packageTemplateBuildFlags = []cli.Flag{
	//
}

func packageTemplateBuild(c *cli.Context) (err error) {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	if len(c.Args()) != 2 {
		cli.ShowCommandHelpAndExit(c, "build", 0)
		return nil
	}

	cacheDir := util.GetConfigDir()

	template := c.Args().Get(0)
	filename := c.Args().Get(1)

	templatePath := filepath.Join(cacheDir, "templates", template)
	if !util.Exists(templatePath) {
		return errors.Errorf("template '%s' does not exist", template)
	}

	if !util.Exists(filename) {
		return errors.Errorf("no such file or directory: %s", filename)
	}

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, templatePath, platform(c), cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "template package is invalid")
	}

	err = util.CopyFile(filename, filepath.Join(templatePath, "tmpl.pwn"))
	if err != nil {
		return errors.Wrap(err, "failed to copy target script to template package directory")
	}

	problems, result, err := pcx.Build(context.Background(), "", false, false, true, "")
	if err != nil {
		return
	}

	print.Info("Build complete with", len(problems), "problems")
	print.Info(fmt.Sprintf(
		"Results, in bytes: Header: %d, Code: %d, Data: %d, Stack/Heap: %d, Estimated usage: %d, Total: %d\n",
		result.Header,
		result.Code,
		result.Data,
		result.StackHeap,
		result.Estimate,
		result.Total,
	))

	return nil
}
