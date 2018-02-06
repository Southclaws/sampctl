package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
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

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		return
	}
	template := c.Args().Get(0)
	filename := c.Args().Get(1)

	templatePath := filepath.Join(cacheDir, "templates", template)
	if !util.Exists(templatePath) {
		return errors.Errorf("template '%s' does not exist", template)
	}

	if !util.Exists(filename) {
		return errors.Errorf("no such file or directory: %s", filename)
	}

	pkg, err := rook.PackageFromDir(true, templatePath, "")
	if err != nil {
		return errors.Wrap(err, "template package is invalid")
	}

	err = util.CopyFile(filename, filepath.Join(templatePath, "tmpl.pwn"))
	if err != nil {
		return errors.Wrap(err, "failed to copy target script to template package directory")
	}

	problems, result, err := rook.Build(context.Background(), gh, gitAuth, &pkg, "", cacheDir, runtime.GOOS, false, false, "")
	if err != nil {
		return
	}

	print.Info("Build complete with", len(problems), "problems")
	print.Info(fmt.Sprintf("Results, in bytes: Header: %d, Code: %d, Data: %d, Stack/Heap: %d, Estimated usage: %d, Total: %d\n",
		result.Header,
		result.Code,
		result.Data,
		result.StackHeap,
		result.Estimate,
		result.Total))

	return
}
