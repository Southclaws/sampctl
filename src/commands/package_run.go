package commands

import (
	"os"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func packageRunFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "server directory",
		},
		cli.BoolFlag{
			Name:  "container",
			Usage: "run the server in a Linux container",
		},
		cli.StringFlag{
			Name:  "build",
			Value: "",
			Usage: "build configuration to use if `--forceBuild` is set",
		},
		cli.BoolFlag{
			Name:  "forceBuild",
			Usage: "build before starting the server",
		},
		cli.BoolFlag{
			Name:  "forceEnsure",
			Usage: "install dependencies before building when `--forceBuild` is set",
		},
		cli.BoolFlag{
			Name:  "noCache",
			Usage: "download plugins again when `--forceEnsure` is set",
		},
		cli.BoolFlag{
			Name:  "watch",
			Usage: "rebuild when source files change",
		},
		cli.StringFlag{
			Name:  "buildFile",
			Value: "",
			Usage: "file for the incrementing build number",
		},
		cli.BoolFlag{
			Name:  "relativePaths",
			Usage: "use relative paths in compiler output",
		},
	}
}

func packageRun(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))
	container := c.Bool("container")
	build := c.String("build")
	forceBuild := c.Bool("forceBuild")
	forceEnsure := c.Bool("forceEnsure")
	noCache := c.Bool("noCache")
	watch := c.Bool("watch")
	buildFile := c.String("buildFile")
	relativePaths := c.Bool("relativePaths")

	runtimeName := c.Args().Get(0)

	pcx, env, err := loadPackageContext(c, dir, false)
	if err != nil {
		return errors.Wrap(err, "failed to load the Pawn package")
	}

	pcx.Runtime = runtimeName
	pcx.Container = container
	pcx.AppVersion = c.App.Version
	pcx.CacheDir = env.CacheDir
	pcx.BuildName = build
	pcx.ForceBuild = forceBuild
	pcx.ForceEnsure = forceEnsure
	pcx.NoCache = noCache
	pcx.BuildFile = buildFile
	pcx.Relative = relativePaths

	ctx, cancel := newCommandContext()
	defer cancel()

	if watch {
		err = pcx.RunWatch(ctx)
	} else {
		err = pcx.Run(ctx, os.Stdout, os.Stdin)
	}

	return reportRunCommandError(c, err)
}
