package commands

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func packageTemplateBuildFlags() []cli.Flag {
	return nil
}

func packageTemplateBuild(c *cli.Context) (err error) {
	if len(c.Args()) != 2 {
		cli.ShowCommandHelpAndExit(c, "build", 0)
		return nil
	}

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}

	template := c.Args().Get(0)
	filename := c.Args().Get(1)

	templatePath := filepath.Join(env.CacheDir, "templates", template)
	if !fs.Exists(templatePath) {
		return errors.Errorf("template '%s' does not exist", template)
	}

	if !fs.Exists(filename) {
		return errors.Errorf("no such file or directory: %s", filename)
	}

	pcx, _, err := loadPackageContext(c, templatePath, false)
	if err != nil {
		return errors.Wrap(err, "template package is invalid")
	}

	err = util.CopyFile(filename, filepath.Join(templatePath, "tmpl.pwn"))
	if err != nil {
		return errors.Wrap(err, "failed to copy the script into the template")
	}

	ctx, cancel := newCommandContext()
	defer cancel()

	problems, result, err := pcx.Build(ctx, pkgcontext.BuildOptions{
		Relative: true,
	})
	if err != nil {
		return
	}

	if !problems.IsValid() {
		return errors.New("template build failed with compiler diagnostics")
	}

	print.Info("Build finished with", len(problems), "compiler diagnostics")
	print.Info(fmt.Sprintf(
		"Size in bytes: header %d, code %d, data %d, stack/heap %d, estimated usage %d, total %d\n",
		result.Header,
		result.Code,
		result.Data,
		result.StackHeap,
		result.Estimate,
		result.Total,
	))

	return nil
}
