package commands

import (
	stderrors "errors"
	"flag"
	"fmt"
	"io"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func init() {
	cli.VersionFlag = cli.BoolFlag{
		Name:  "appVersion, V, version",
		Usage: "sampctl version",
	}
}

func Run(args []string, version string) error {
	checking := isCheckInvocation(args)
	cacheDir := fs.ConfigDirPath()
	if !checking {
		var err error
		cacheDir, err = fs.ConfigDir()
		if err != nil {
			return errors.Wrap(err, "failed to find the sampctl config directory")
		}

		if err := download.MigrateOldConfig(cacheDir); err != nil {
			return errors.Wrap(err, "failed to move the old sampctl config directory")
		}
	}

	state := newCommandState(version, cacheDir)
	app := newCLIApp(version, state)
	if err := app.Run(args); err != nil {
		if checking {
			var exitErr *commandExitError
			if !stderrors.As(err, &exitErr) {
				return &commandExitError{code: 2, err: err}
			}
		}
		return err
	}

	if checking {
		return nil
	}

	return state.saveConfig()
}

func isCheckInvocation(args []string) bool {
	if len(args) < 2 {
		return false
	}

	set := flag.NewFlagSet("sampctl", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	flags := withGlobalFlags(globalFlags(), []cli.Flag{cli.HelpFlag, cli.VersionFlag, cli.BashCompletionFlag})
	for _, f := range flags {
		f.Apply(set)
	}

	if err := set.Parse(args[1:]); err != nil {
		return isCheckCommand(set.Args())
	}

	return isCheckCommand(cli.Args(set.Args()))
}

func isCheckCommand(args cli.Args) bool {
	return args.First() == "check" || args.First() == "help" && args.Get(1) == "check"
}

func newCLIApp(version string, state *commandState) *cli.App {
	app := cli.NewApp()
	app.Metadata = map[string]interface{}{commandStateKey: state}
	app.Authors = []cli.Author{
		{Name: "Southclaws", Email: "hello@southcla.ws"},
		{Name: "JustMichael", Email: "michael@sag.gs"},
	}
	app.Name = "sampctl"
	app.Usage = "Manage dependencies, build Pawn scripts, and run SA-MP or open.mp servers."
	app.Version = version
	app.EnableBashCompletion = true

	global := globalFlags()
	app.Flags = global
	app.Commands = appCommands(global)
	app.Before = state.configure
	app.After = func(c *cli.Context) error {
		if isCheckCommand(c.Args()) {
			return nil
		}

		state.maybeCheckForUpdates(c)

		return nil
	}
	app.OnUsageError = func(_ *cli.Context, err error, _ bool) error {
		return err
	}

	return app
}

func globalFlags() []cli.Flag {
	return []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "show detailed logs and error causes",
		},
		cli.StringFlag{
			Name:  "platform",
			Value: "",
			Usage: "target platform for downloaded binaries: windows, linux, or darwin",
		},
		cli.BoolFlag{
			Name:  "bare",
			Usage: "skip all pre-run configuration",
		},
	}
}

func appCommands(global []cli.Flag) []cli.Command {
	return []cli.Command{
		newInitCommand(global),
		newEnsureCommand(global),
		newInstallCommand(global),
		newUninstallCommand(global),
		newReleaseCommand(global),
		newConfigCommand(global),
		newGetCommand(global),
		newBuildCommand(global),
		newRunCommand(global),
		newCheckCommand(global),
		newCompilerCommand(global),
		newTemplateCommand(global),
		newVersionCommand(),
		newCompletionCommand(),
		newDocsCommand(),
	}
}

func withGlobalFlags(global, local []cli.Flag) []cli.Flag {
	flags := make([]cli.Flag, 0, len(global)+len(local))
	flags = append(flags, global...)
	flags = append(flags, local...)
	return flags
}

func newInitCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "init",
		Usage:       "sampctl init [--preset samp|openmp]",
		Description: "Create a SA-MP or open.mp project.",
		Action:      packageInit,
		Flags:       withGlobalFlags(global, packageInitFlags()),
	}
}

func newEnsureCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "ensure",
		Usage:       "sampctl ensure [dependency]",
		Description: "Install dependencies declared in `pawn.json`/`pawn.yaml`.",
		Action:      packageEnsure,
		Flags:       withGlobalFlags(global, packageEnsureFlags()),
	}
}

func newInstallCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:         "install",
		Usage:        "sampctl install [package definition]",
		Description:  "Add dependencies to `pawn.json`/`pawn.yaml` and install them.",
		Action:       packageInstall,
		Flags:        withGlobalFlags(global, packageInstallFlags()),
		BashComplete: packageInstallBash,
	}
}

func newUninstallCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "uninstall",
		Usage:       "sampctl uninstall [package definition]",
		Description: "Remove dependencies from `pawn.json`/`pawn.yaml` and update an existing pawn.lock. Installed files remain.",
		Action:      packageUninstall,
		Flags:       withGlobalFlags(global, packageUninstallFlags()),
	}
}

func newReleaseCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "release",
		Usage:       "sampctl release",
		Description: "Tag the next version and create a GitHub release with archived package files.",
		Action:      packageRelease,
		Flags:       withGlobalFlags(global, packageReleaseFlags()),
	}
}

func newConfigCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "config",
		Usage:       "sampctl config [field] [value]",
		Description: "View or change sampctl settings.",
		Action:      packageConfig,
		Flags:       withGlobalFlags(global, packageConfigFlags()),
	}
}

func newGetCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:         "get",
		Usage:        "sampctl get [package definition] (target path)",
		Description:  "Clone a GitHub package and install its dependencies. Use the current directory if empty, or a directory named after the repository.",
		Action:       packageGet,
		Flags:        withGlobalFlags(global, packageGetFlags()),
		BashComplete: packageGetBash,
	}
}

func newBuildCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:         "build",
		Usage:        "sampctl build [build name]",
		Description:  "Build a package defined in `pawn.json`/`pawn.yaml`.",
		Action:       packageBuild,
		Flags:        withGlobalFlags(global, packageBuildFlags()),
		BashComplete: packageBuildBash,
	}
}

func newRunCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "run",
		Usage:       "sampctl run",
		Description: "Compile and run a package defined in `pawn.json`/`pawn.yaml`.",
		Action:      packageRun,
		Flags:       withGlobalFlags(global, packageRunFlags()),
	}
}

func newCompilerCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "compiler",
		Usage:       "sampctl compiler",
		Description: "Manage compiler settings.",
		Subcommands: []cli.Command{
			{
				Name:        "list",
				Usage:       "sampctl compiler list",
				Description: "List compiler presets and their settings.",
				Action:      compilerList,
				Flags:       withGlobalFlags(global, compilerFlags()),
			},
		},
	}
}

func newTemplateCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "template",
		Usage:       "sampctl template <subcommand>",
		Description: "Create and use package templates.",
		Subcommands: []cli.Command{
			{
				Name:        "make",
				Usage:       "sampctl template make [name]",
				Description: "Create a package template.",
				Action:      packageTemplateMake,
				Flags:       withGlobalFlags(global, packageTemplateMakeFlags()),
			},
			{
				Name:        "build",
				Usage:       "sampctl template build [template] [filename]",
				Description: "Build a script using a package template.",
				Action:      packageTemplateBuild,
				Flags:       withGlobalFlags(global, packageTemplateBuildFlags()),
			},
			{
				Name:        "run",
				Usage:       "sampctl template run [template] [filename]",
				Description: "Build and run a script using a package template.",
				Action:      packageTemplateRun,
				Flags:       withGlobalFlags(global, packageTemplateRunFlags()),
			},
		},
	}
}

func newVersionCommand() cli.Command {
	return cli.Command{
		Name:        "version",
		Description: "Show the sampctl version, also used for `--container` images.",
		Action:      cli.VersionPrinter,
	}
}

func newCompletionCommand() cli.Command {
	return cli.Command{
		Name:        "completion",
		Description: "Install shell completion.",
		Action:      autoComplete,
	}
}

func newDocsCommand() cli.Command {
	return cli.Command{
		Name:        "docs",
		Usage:       "sampctl docs > documentation.md",
		Description: "Print command documentation as Markdown.",
		Action: func(c *cli.Context) error {
			fmt.Print(GenerateDocs(c.App))
			return nil
		},
	}
}

func platform(c *cli.Context) (platform string) {
	platform = c.String("platform")
	if platform == "" {
		platform = runtime.GOOS
	}
	return
}
