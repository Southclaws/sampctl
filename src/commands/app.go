package commands

import (
	"fmt"
	"runtime"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func Run(args []string, version string) error {
	cacheDir, err := fs.ConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config dir")
	}

	if err := download.MigrateOldConfig(cacheDir); err != nil {
		return errors.Wrap(err, "failed to migrate old config directory to new config directory")
	}

	state := newCommandState(version, cacheDir)
	app := newCLIApp(version, state)
	if err := app.Run(args); err != nil {
		return err
	}

	return state.saveConfig()
}

func newCLIApp(version string, state *commandState) *cli.App {
	app := cli.NewApp()
	app.Metadata = map[string]interface{}{commandStateKey: state}
	app.Authors = []cli.Author{
		{Name: "Southclaws", Email: "hello@southcla.ws"},
		{Name: "JustMichael", Email: "michael@sag.gs"},
	}
	app.Name = "sampctl"
	app.Usage = "The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer."
	app.Version = version
	app.EnableBashCompletion = true

	cli.VersionFlag = cli.BoolFlag{
		Name:  "appVersion, V, version",
		Usage: "sampctl version",
	}

	global := globalFlags()
	app.Flags = global
	app.Commands = appCommands(global)
	app.Before = state.configure
	app.After = func(c *cli.Context) error {
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
			Usage: "output all detailed information - useful for debugging",
		},
		cli.StringFlag{
			Name:  "platform",
			Value: "",
			Usage: "manually specify the target platform for downloaded binaries to either `windows`, `linux` or `darwin`.",
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
		Description: "Initialises a new samp or open.mp project.",
		Action:      packageInit,
		Flags:       withGlobalFlags(global, packageInitFlags()),
	}
}

func newEnsureCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "ensure",
		Usage:       "sampctl ensure",
		Description: "Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`/`pawn.yaml`.",
		Action:      packageEnsure,
		Flags:       withGlobalFlags(global, packageEnsureFlags()),
	}
}

func newInstallCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:         "install",
		Usage:        "sampctl install [package definition]",
		Description:  "Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` and downloads the contents.",
		Action:       packageInstall,
		Flags:        withGlobalFlags(global, packageInstallFlags()),
		BashComplete: packageInstallBash,
	}
}

func newUninstallCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "uninstall",
		Usage:       "sampctl uninstall [package definition]",
		Description: "Uninstalls package by removing it from the `dependencies` field in `pawn.json`/`pawn.yaml` and deletes the contents.",
		Action:      packageUninstall,
		Flags:       withGlobalFlags(global, packageUninstallFlags()),
	}
}

func newReleaseCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "release",
		Usage:       "sampctl release",
		Description: "Creates a release version and tags the repository with the next version number, creates a GitHub release with archived package files.",
		Action:      packageRelease,
		Flags:       withGlobalFlags(global, packageReleaseFlags()),
	}
}

func newConfigCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "config",
		Usage:       "configure config options",
		Description: "Allows configuring the field values for the config",
		Action:      packageConfig,
		Flags:       withGlobalFlags(global, packageConfigFlags()),
	}
}

func newGetCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:         "get",
		Usage:        "sampctl get [package definition] (target path)",
		Description:  "Clones a GitHub package to either a directory named after the repo or, if the cwd is empty, the cwd and then ensures the package.",
		Action:       packageGet,
		Flags:        withGlobalFlags(global, packageGetFlags()),
		BashComplete: packageGetBash,
	}
}

func newBuildCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:         "build",
		Usage:        "sampctl build [build name]",
		Description:  "Builds a package defined by a `pawn.json`/`pawn.yaml` file.",
		Action:       packageBuild,
		Flags:        withGlobalFlags(global, packageBuildFlags()),
		BashComplete: packageBuildBash,
	}
}

func newRunCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "run",
		Usage:       "sampctl run",
		Description: "Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.",
		Action:      packageRun,
		Flags:       withGlobalFlags(global, packageRunFlags()),
	}
}

func newCompilerCommand(global []cli.Flag) cli.Command {
	return cli.Command{
		Name:        "compiler",
		Usage:       "sampctl compiler",
		Description: "Provides commands for managing compiler configurations",
		Subcommands: []cli.Command{
			{
				Name:        "list",
				Usage:       "sampctl compiler list",
				Description: "Lists available compiler presets and their configurations.",
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
		Description: "Provides commands for package templates",
		Subcommands: []cli.Command{
			{
				Name:        "make",
				Usage:       "sampctl template make [name]",
				Description: "Creates a template package from the current directory if it is a package.",
				Action:      packageTemplateMake,
				Flags:       withGlobalFlags(global, packageTemplateMakeFlags()),
			},
			{
				Name:        "build",
				Usage:       "sampctl template build [template] [filename]",
				Description: "Builds the specified file in the context of the given template.",
				Action:      packageTemplateBuild,
				Flags:       withGlobalFlags(global, packageTemplateBuildFlags()),
			},
			{
				Name:        "run",
				Usage:       "sampctl template run [template] [filename]",
				Description: "Builds and runs the specified file in the context of the given template.",
				Action:      packageTemplateRun,
				Flags:       withGlobalFlags(global, packageTemplateRunFlags()),
			},
		},
	}
}

func newVersionCommand() cli.Command {
	return cli.Command{
		Name:        "version",
		Description: "Show version number - this is also the version of the container image that will be used for `--container` runtimes.",
		Action:      cli.VersionPrinter,
	}
}

func newCompletionCommand() cli.Command {
	return cli.Command{
		Name:        "completion",
		Description: "output bash autocomplete code",
		Action:      autoComplete,
	}
}

func newDocsCommand() cli.Command {
	return cli.Command{
		Name:        "docs",
		Usage:       "sampctl docs > documentation.md",
		Description: "Generate documentation in markdown format and print to standard out.",
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
