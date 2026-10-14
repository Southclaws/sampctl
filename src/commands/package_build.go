package commands

import (
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func packageBuildFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "project directory",
		},
		cli.BoolFlag{
			Name:  "forceEnsure",
			Usage: "install dependencies before building",
		},
		cli.BoolFlag{
			Name:  "dryRun",
			Usage: "print the build command without running it",
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
		cli.BoolFlag{
			Name:  "no-lock",
			Usage: "disable lockfile support",
		},
	}
}

func packageBuild(c *cli.Context) error {
	dir := fs.MustAbs(c.String("dir"))
	forceEnsure := c.Bool("forceEnsure")
	dryRun := c.Bool("dryRun")
	watch := c.Bool("watch")
	buildFile := c.String("buildFile")
	relativePaths := c.Bool("relativePaths")
	noLock := c.Bool("no-lock")
	useLockfile := !noLock

	build := c.Args().Get(0)

	pcx, _, err := loadPackageContextWithCache(c, dir, false, true, useLockfile)
	if err != nil {
		return errors.Wrap(err, "failed to load the Pawn package")
	}

	ctx, cancel := newCommandContext()
	defer cancel()

	if watch {
		err := pcx.BuildWatch(ctx, pkgcontext.BuildOptions{
			Name:      build,
			Ensure:    forceEnsure,
			BuildFile: buildFile,
			Relative:  relativePaths,
		})
		if err != nil {
			return reportDependencyCommandError(c, "build", err)
		}
	} else {
		problems, result, err := pcx.Build(ctx, pkgcontext.BuildOptions{
			Name:      build,
			Ensure:    forceEnsure,
			DryRun:    dryRun,
			Relative:  relativePaths,
			BuildFile: buildFile,
		})
		if err != nil {
			return reportDependencyCommandError(c, "build", err)
		}

		if build == "" {
			build = "default"
		}

		if problems.Fatal() {
			return cli.NewExitError(errors.New("build stopped after a fatal compiler error"), 1)
		} else if len(problems.Errors()) > 0 {
			return cli.NewExitError(errors.Errorf("build failed with %d compiler diagnostics", len(problems)), 1)
		} else if len(problems.Warnings()) > 0 {
			print.Warn("Build", build, "finished with", len(problems), "compiler diagnostics")
		} else {
			print.Info("Build", build, "finished with", len(problems), "compiler diagnostics")
		}

		print.Verb(fmt.Sprintf(
			"Size in bytes: header %d, code %d, data %d, stack/heap %d, estimated usage %d, total %d\n",
			result.Header,
			result.Code,
			result.Data,
			result.StackHeap,
			result.Estimate,
			result.Total,
		))

		if useLockfile && !problems.Fatal() && len(problems.Errors()) == 0 {
			config := pcx.ActualBuild
			if saveErr := persistBuildLockfile(
				pcx,
				config.Compiler.Version,
				config.Compiler.Preset,
				config.Input,
				config.Output,
			); saveErr != nil {
				print.Warn("failed to save lockfile:", saveErr)
			}
		}
	}

	return nil
}

func packageBuildBash(c *cli.Context) {
	dir := fs.MustAbs(c.String("dir"))
	pcx, _, err := loadPackageContext(c, dir, false)
	if err != nil {
		return
	}

	for _, b := range pcx.Package.Builds {
		fmt.Println(b.Name)
	}
}
