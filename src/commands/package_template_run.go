package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

var packageTemplateRunFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "version",
		Value: "0.3.7",
		Usage: "the SA:MP server version to use",
	},
	cli.StringFlag{
		Name:  "mode",
		Value: "main",
		Usage: "runtime mode, one of: server, main, y_testing",
	},
}

func packageTemplateRun(c *cli.Context) (err error) {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	version := c.String("version")
	mode := c.String("mode")

	if len(c.Args()) != 2 {
		cli.ShowCommandHelpAndExit(c, "run", 0)
		return nil
	}

	cacheDir := download.GetCacheDir()
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

	// override the version with the one passed by --version
	pcx.Package.Runtime.Version = version

	if !problems.IsValid() {
		return errors.New("cannot run with build errors")
	}
	pcx.Runtime = ""
	pcx.Container = false
	pcx.AppVersion = c.App.Version
	pcx.CacheDir = cacheDir
	pcx.BuildName = ""
	pcx.ForceBuild = false
	pcx.ForceEnsure = false
	pcx.NoCache = false
	pcx.BuildFile = ""
	pcx.Relative = false

	pcx.Package.Runtime = new(run.Runtime)
	pcx.Package.Runtime.Mode = run.RunMode(mode)

	err = pcx.Run(context.Background(), os.Stdout, os.Stdin)
	if err != nil {
		return
	}

	return nil
}
