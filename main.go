package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/segmentio/analytics-go.v3"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
)

var (
	version    = "master"
	segmentKey = ""
	config     *types.Config        // global config
	gh         *github.Client       // a github client to use for API requests
	gitAuth    transport.AuthMethod // for private dependencies
	segment    analytics.Client     // segment.io client
)

func main() {
	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return
	}

	app := cli.NewApp()

	app.Author = "Southclaws"
	app.Email = "hello@southcla.ws"
	app.Name = "sampctl"
	app.Description = "The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer."
	app.Version = version
	app.EnableBashCompletion = true

	cli.VersionFlag = cli.BoolFlag{
		Name:  "appVersion, V",
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
	app.Commands = []cli.Command{
		{
			Name:        "server",
			Aliases:     []string{"s"},
			Usage:       "sampctl server <subcommand>",
			Description: "For managing servers and runtime configurations.",
			Subcommands: []cli.Command{
				{
					Name:        "init",
					Usage:       "sampctl server init",
					Description: "Bootstrap a new SA:MP server and generates a `samp.json`/`samp.yaml` configuration based on user input. If `gamemodes`, `filterscripts` or `plugins` directories are present, you will be prompted to select relevant files.",
					Action:      serverInit,
					Flags:       append(globalFlags, serverInitFlags...),
				},
				{
					Name:        "download",
					Usage:       "sampctl server download",
					Description: "Downloads the files necessary to run a SA:MP server to the current directory (unless `--dir` specified). Will download the latest stable (non RC) server version unless `--version` is specified.",
					Action:      serverDownload,
					Flags:       append(globalFlags, serverDownloadFlags...),
				},
				{
					Name:        "ensure",
					Usage:       "sampctl server ensure",
					Description: "Ensures the server environment is representative of the configuration specified in `samp.json`/`samp.yaml` - downloads server binaries and plugin files if necessary and generates a `server.cfg` file.",
					Action:      serverEnsure,
					Flags:       append(globalFlags, serverEnsureFlags...),
				},
				{
					Name:        "run",
					Usage:       "sampctl server run",
					Description: "Generates a `server.cfg` file based on the configuration inside `samp.json`/`samp.yaml` then executes the server process and automatically restarts it on crashes.",
					Action:      serverRun,
					Flags:       append(globalFlags, serverRunFlags...),
				},
			},
		},
		{
			Name:        "package",
			Aliases:     []string{"p"},
			Usage:       "sampctl package <subcommand>",
			Description: "For managing Pawn packages such as gamemodes and libraries.",
			Subcommands: []cli.Command{
				{
					Name:        "init",
					Usage:       "sampctl package init",
					Description: "Helper tool to bootstrap a new package or turn an existing project into a package.",
					Action:      packageInit,
					Flags:       append(globalFlags, packageInitFlags...),
				},
				{
					Name:        "ensure",
					Usage:       "sampctl package ensure",
					Description: "Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`/`pawn.yaml`.",
					Action:      packageEnsure,
					Flags:       append(globalFlags, packageEnsureFlags...),
				},
				{
					Name:         "install",
					Usage:        "sampctl package install [package definition]",
					Description:  "Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` and downloads the contents.",
					Action:       packageInstall,
					Flags:        append(globalFlags, packageInstallFlags...),
					BashComplete: packageInstallBash,
				},
				{
					Name:         "uninstall",
					Usage:        "sampctl package uninstall [package definition]",
					Description:  "Uninstalls package by removing it from the `dependencies` field in `pawn.json`/`pawn.yaml` and deletes the contents.",
					Action:       packageUninstall,
					Flags:        append(globalFlags, packageUninstallFlags...),
					BashComplete: packageUninstallBash,
				},
				{
					Name:        "release",
					Usage:       "sampctl package release",
					Description: "Creates a release version and tags the repository with the next version number, creates a GitHub release with archived package files.",
					Action:      packageRelease,
					Flags:       append(globalFlags, packageReleaseFlags...),
				},
				{
					Name:         "get",
					Usage:        "sampctl package get [package definition] (target path)",
					Description:  "Clones a GitHub package to either a directory named after the repo or, if the cwd is empty, the cwd and then ensures the package.",
					Action:       packageGet,
					Flags:        append(globalFlags, packageGetFlags...),
					BashComplete: packageGetBash,
				},
				{
					Name:         "build",
					Usage:        "sampctl package build [build name]",
					Description:  "Builds a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:       packageBuild,
					Flags:        append(globalFlags, packageBuildFlags...),
					BashComplete: packageBuildBash,
				},
				{
					Name:        "run",
					Usage:       "sampctl package run",
					Description: "Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:      packageRun,
					Flags:       append(globalFlags, packageRunFlags...),
				},
				{
					Name:        "template",
					Usage:       "sampctl package template <subcommand>",
					Description: "Provides commands for package templates",
					Subcommands: []cli.Command{
						{
							Name:        "make",
							Usage:       "sampctl package template make [name]",
							Description: "Creates a template package from the current directory if it is a package.",
							Action:      packageTemplateMake,
							Flags:       append(globalFlags, packageTemplateMakeFlags...),
						},
						{
							Name:        "build",
							Usage:       "sampctl package template build [template] [filename]",
							Description: "Builds the specified file in the context of the given template.",
							Action:      packageTemplateBuild,
							Flags:       append(globalFlags, packageTemplateBuildFlags...),
						},
						{
							Name:        "run",
							Usage:       "sampctl package template run [template] [filename]",
							Description: "Builds and runs the specified file in the context of the given template.",
							Action:      packageTemplateRun,
							Flags:       append(globalFlags, packageTemplateRunFlags...),
						},
					},
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

		config, err = types.LoadOrCreateConfig(cacheDir, verbose)
		if err != nil {
			return errors.Wrapf(err, "Failed to load or create sampctl config in %s", cacheDir)
		}

		if config.GitHubToken == "" {
			gh = github.NewClient(nil)
		} else {
			gh = github.NewClient(oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.GitHubToken})))
		}

		if config.GitUsername != "" && config.GitPassword != "" {
			gitAuth = http.NewBasicAuth(config.GitUsername, config.GitPassword)
		} else {
			gitAuth, err = ssh.DefaultAuthBuilder("git")
			if err != nil {
				print.Verb("Failed to set up SSH:", err)
			}
		}

		if config.CI != "" {
			config.Metrics = false
		}
		if segmentKey == "" {
			print.Warn("Segment.io key is unset!")
			config.Metrics = false
		}

		if config.Metrics {
			segment = analytics.New(segmentKey)
			if config.NewUser {
				print.Info("Usage metrics are active. See https://github.com/Southclaws/sampctl/wiki/Usage-Metrics for more information.")
				segment.Enqueue(analytics.Identify{
					UserId: config.UserID,
				})
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
		if !c.GlobalIsSet("generate-bash-completion") &&
			!c.GlobalIsSet("bare") &&
			time.Now().Minute()%2 == 0 &&
			time.Now().Second()%2 == 0 {
			CheckForUpdates(app.Version)
		}
		return nil
	}

	err = app.Run(os.Args)
	if err != nil {
		print.Erro(err)
	}

	if config != nil {
		err = types.WriteConfig(cacheDir, *config)
		if err != nil {
			print.Erro("Failed to write updated configuration file to", cacheDir, "-", err)
			return
		}
	}

	if segment != nil {
		segment.Close()
	}
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
