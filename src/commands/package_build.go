package commands

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

var packageBuildFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "dir",
		Value: ".",
		Usage: "working directory for the project - by default, uses the current directory",
	},
	cli.BoolFlag{
		Name:  "forceEnsure",
		Usage: "forces dependency ensure before build",
	},
	cli.BoolFlag{
		Name:  "dryRun",
		Usage: "does not run the build but outputs the command necessary to do so",
	},
	cli.BoolFlag{
		Name:  "watch",
		Usage: "keeps sampctl running and triggers builds whenever source files change",
	},
	cli.StringFlag{
		Name:  "buildFile",
		Value: "",
		Usage: "declares a file to store the incrementing build number for easy versioning",
	},
	cli.BoolFlag{
		Name:  "relativePaths",
		Usage: "force compiler output to use relative paths instead of absolute",
	},
	cli.BoolFlag{
		Name:  "no-lock",
		Usage: "disable lockfile support",
	},
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

	env, err := getCommandEnv(c)
	if err != nil {
		return err
	}

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, env.Platform, env.CacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to interpret directory as Pawn package")
	}

	if useLockfile {
		err = pcx.InitLockfileResolver(sampctlVersion)
		if err != nil {
			print.Warn("failed to initialize lockfile resolver:", err)
		}
	}

	if watch {
		err := pcx.BuildWatch(context.Background(), build, forceEnsure, buildFile, relativePaths, nil)
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}
	} else {
		problems, result, err := pcx.Build(context.Background(), build, forceEnsure, dryRun, relativePaths, buildFile)
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}

		if build == "" {
			build = "default"
		}

		if problems.Fatal() {
			return cli.NewExitError(errors.New("Build encountered fatal error"), 1)
		} else if len(problems.Errors()) > 0 {
			return cli.NewExitError(errors.Errorf("Build failed with %d problems", len(problems)), 1)
		} else if len(problems.Warnings()) > 0 {
			print.Warn("Build", build, "complete with", len(problems), "problems")
		} else {
			print.Info("Build", build, "successful with", len(problems), "problems")
		}

		print.Verb(fmt.Sprintf(
			"Results, in bytes: Header: %d, Code: %d, Data: %d, Stack/Heap: %d, Estimated usage: %d, Total: %d\n",
			result.Header,
			result.Code,
			result.Data,
			result.StackHeap,
			result.Estimate,
			result.Total,
		))

		if useLockfile && !problems.Fatal() && len(problems.Errors()) == 0 {
			config := pcx.Package.GetBuildConfig(build)
			if config != nil {
				pcx.RecordBuildToLockfile(
					config.Compiler.Version,
					config.Compiler.Preset,
					pcx.Package.Entry,
					pcx.Package.Output,
				)
				if saveErr := pcx.SaveLockfile(); saveErr != nil {
					print.Warn("failed to save lockfile:", saveErr)
				}
			}
		}
	}

	return nil
}

func packageBuildBash(c *cli.Context) {
	dir := fs.MustAbs(c.String("dir"))
	env, err := getCommandEnv(c)
	if err != nil {
		return
	}

	pcx, err := pkgcontext.NewPackageContext(gh, gitAuth, true, dir, env.Platform, env.CacheDir, "", false)
	if err != nil {
		return
	}

	for _, b := range pcx.Package.Builds {
		fmt.Println(b.Name)
	}
}
