package commands

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/Masterminds/semver"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"github.com/kirsle/configdir"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
)

var (
	cfg     *config.Config       // global config
	gh      *github.Client       // a github client to use for API requests
	gitAuth transport.AuthMethod // for private dependencies
)

func Run(args []string, version string) error {
	cacheDir := util.GetConfigDir()
	err := configdir.MakePath(cacheDir)
	if err != nil {
		return errors.Wrap(err, "Failed to create config path")
	}

	err = download.MigrateOldConfig(cacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to migrate old config directory to new config directory")
	}

	app := cli.NewApp()

	app.Authors = []cli.Author{
		{
			Name:  "Southclaws",
			Email: "hello@southcla.ws",
		},
		{
			Name:  "JustMichael",
			Email: "michael@sag.gs",
		},
	}
	app.Name = "sampctl"
	app.Usage = "The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer."
	app.Version = version
	app.EnableBashCompletion = true

	cli.VersionFlag = cli.BoolFlag{
		Name:  "appVersion, V, version",
		Usage: "sampctl version",
	}

	globalFlags := []cli.Flag{
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
	//nolint:lll
	app.Commands = []cli.Command{
		{
			Name:        "init",
			Usage:       "sampctl init [--runtime samp|openmp]",
			Description: "Initialises a new samp or open.mp project.",
			Action:      packageInit,
			Flags:       append(globalFlags, packageInitFlags...),
		},
		{
			Name:        "ensure",
			Usage:       "sampctl ensure",
			Description: "Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`/`pawn.yaml`.",
			Action:      packageEnsure,
			Flags:       append(globalFlags, packageEnsureFlags...),
		},
		{
			Name:         "install",
			Usage:        "sampctl install [package definition]",
			Description:  "Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` and downloads the contents.",
			Action:       packageInstall,
			Flags:        append(globalFlags, packageInstallFlags...),
			BashComplete: packageInstallBash,
		},
		{
			Name:        "uninstall",
			Usage:       "sampctl uninstall [package definition]",
			Description: "Uninstalls package by removing it from the `dependencies` field in `pawn.json`/`pawn.yaml` and deletes the contents.",
			Action:      packageUninstall,
			Flags:       append(globalFlags, packageUninstallFlags...),
		},
		{
			Name:        "release",
			Usage:       "sampctl release",
			Description: "Creates a release version and tags the repository with the next version number, creates a GitHub release with archived package files.",
			Action:      packageRelease,
			Flags:       append(globalFlags, packageReleaseFlags...),
		},
		{
			Name:        "config",
			Usage:       "configure config options",
			Description: "Allows configuring the field values for the config",
			Action:      packageConfig,
			Flags:       append(globalFlags, packageConfigFlags...),
		},
		{
			Name:         "get",
			Usage:        "sampctl get [package definition] (target path)",
			Description:  "Clones a GitHub package to either a directory named after the repo or, if the cwd is empty, the cwd and then ensures the package.",
			Action:       packageGet,
			Flags:        append(globalFlags, packageGetFlags...),
			BashComplete: packageGetBash,
		},
		{
			Name:         "build",
			Usage:        "sampctl build [build name]",
			Description:  "Builds a package defined by a `pawn.json`/`pawn.yaml` file.",
			Action:       packageBuild,
			Flags:        append(globalFlags, packageBuildFlags...),
			BashComplete: packageBuildBash,
		},
		{
			Name:        "run",
			Usage:       "sampctl run",
			Description: "Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.",
			Action:      packageRun,
			Flags:       append(globalFlags, packageRunFlags...),
		},
		{
			Name:        "compiler",
			Usage:       "sampctl compiler",
			Description: "Provides commands for managing compiler configurations",
			Subcommands: []cli.Command{
				{
					Name:        "list",
					Usage:       "sampctl compiler list",
					Description: "Lists available compiler presets and their configurations.",
					Action:      compilerList,
					Flags:       append(globalFlags, compilerFlags...),
				},
			},
		},
		{
			Name:        "template",
			Usage:       "sampctl template <subcommand>",
			Description: "Provides commands for package templates",
			Subcommands: []cli.Command{
				{
					Name:        "make",
					Usage:       "sampctl template make [name]",
					Description: "Creates a template package from the current directory if it is a package.",
					Action:      packageTemplateMake,
					Flags:       append(globalFlags, packageTemplateMakeFlags...),
				},
				{
					Name:        "build",
					Usage:       "sampctl template build [template] [filename]",
					Description: "Builds the specified file in the context of the given template.",
					Action:      packageTemplateBuild,
					Flags:       append(globalFlags, packageTemplateBuildFlags...),
				},
				{
					Name:        "run",
					Usage:       "sampctl template run [template] [filename]",
					Description: "Builds and runs the specified file in the context of the given template.",
					Action:      packageTemplateRun,
					Flags:       append(globalFlags, packageTemplateRunFlags...),
				},
			},
		},
		{
			Name:        "version",
			Description: "Show version number - this is also the version of the container image that will be used for `--container` runtimes.",
			Action:      cli.VersionPrinter,
		},
		{
			Name:        "completion",
			Description: "output bash autocomplete code",
			Action:      autoComplete,
		},
		{
			Name:        "docs",
			Usage:       "sampctl docs > documentation.md",
			Description: "Generate documentation in markdown format and print to standard out.",
			Action: func(c *cli.Context) error {
				docs := GenerateDocs(c.App)
				fmt.Print(docs)
				return nil
			},
		},
	}

	app.Flags = globalFlags
	app.Before = func(c *cli.Context) error {
		err = godotenv.Load(".env")
		if err != nil {
			print.Verb(err)
		}

		verbose := c.GlobalBool("verbose")

		// "bare" mode is for CI use only
		if c.GlobalBool("bare") {
			return nil
		}

		if verbose {
			print.SetVerbose()
			print.Verb("Verbose logging active")
		}
		if runtime.GOOS != "windows" {
			print.SetColoured()
		}

		cfg, err = config.LoadOrCreateConfig(cacheDir, verbose)
		if err != nil {
			return errors.Wrapf(err, "Failed to load or create sampctl config in %s", cacheDir)
		}

		if cfg.GitHubToken == "" {
			gh = github.NewClient(nil)
		} else {
			gh = github.NewClient(
				oauth2.NewClient(context.Background(),
					oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.GitHubToken})),
			)
		}

		if cfg.GitUsername != "" && cfg.GitPassword != "" {
			gitAuth = &http.BasicAuth{
				Username: cfg.GitUsername,
				Password: cfg.GitPassword,
			}
		} else {
			gitAuth, err = ssh.DefaultAuthBuilder("git")
			if err != nil {
				print.Verb("Failed to set up SSH:", err)
			}
		}

		return nil
	}
	app.After = func(c *cli.Context) error {
		if c.GlobalIsSet("generate-bash-completion") {
			return nil
		}

		// quick and dirty stateless check to make sure update check doesn't run on *every* execution
		// instead, it will only check when the user happens to run the app during a minute and second
		// that are even numbers. 12:56:44 will work, 12:57:44 will not, etc...
		// this is done because the GitHub API has rate limits and we don't want to use all our requests
		// up on version checks when package management is more important.
		if !*cfg.HideVersionUpdateMessage {
			if !c.GlobalIsSet("generate-bash-completion") &&
				!c.GlobalIsSet("bare") &&
				time.Now().Minute()%2 == 0 &&
				time.Now().Second()%2 == 0 {
				CheckForUpdates(app.Version)
			}
		}
		return nil
	}
	app.OnUsageError = func(c *cli.Context, err error, isSubcommand bool) error {
		return err
	}

	err = app.Run(args)
	if err != nil {
		return err
	}

	if cfg != nil {
		err = config.WriteConfig(cacheDir, *cfg)
		if err != nil {
			return errors.Errorf("Failed to write updated configuration file to %s - %v", cacheDir, err)
		}
	}

	return nil
}

// CheckForUpdates uses the GitHub API to check if a new release is available.
func CheckForUpdates(thisVersion string) {
	if gh == nil {
		return
	}

	ctx, cf := context.WithTimeout(context.Background(), time.Second*10)
	defer cf()

	release, _, err := gh.Repositories.GetLatestRelease(ctx, "Southclaws", "sampctl")
	if err != nil {
		print.Erro("Failed to check for latest sampctl release:", err)
		return
	}

	latest, err := semver.NewVersion(release.GetTagName())
	if err != nil {
		print.Erro("Failed to interpret latest release tag as a semantic version:", err)
		return
	}

	this, err := semver.NewVersion(thisVersion)
	if err != nil {
		print.Erro("Failed to interpret this version number as a semantic version:", err)
		return
	}

	if latest.GreaterThan(this) {
		print.Info("\n-\n")
		print.Info("sampctl version", latest.String(), "available!")
		print.Info("You are currently using", thisVersion)
		print.Info("To upgrade, use the following command:")
		switch runtime.GOOS {
		case "windows":
			print.Info("  scoop update")
			print.Info("  scoop update sampctl")
		case "linux":
			print.Info("  Debian/Ubuntu based systems:")
			print.Info("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/install-deb.sh | sh")
			print.Info("  CentOS/Red Hat based systems")
			print.Info("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/install-rpm.sh | sh")
		case "darwin":
			print.Info("  brew update")
			print.Info("  brew upgrade sampctl")
		}
		print.Info("If you have any problems upgrading, please open an issue:")
		print.Info("  https://github.com/Southclaws/sampctl/issues/new")
	}
}

func platform(c *cli.Context) (platform string) {
	platform = c.String("platform")
	if platform == "" {
		platform = runtime.GOOS
	}
	return
}
