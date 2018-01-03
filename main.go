package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Masterminds/semver"
	"github.com/fatih/color"
	"github.com/google/go-github/github"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
)

var version = "master"

func main() {
	app := cli.NewApp()

	app.Author = "Southclaws"
	app.Email = "southclaws@gmail.com"
	app.Name = "sampctl"
	app.Usage = "The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer."
	app.Description = "Compiles server configuration JSON to server.cfg format. Executes the server and monitors it for crashes, restarting if necessary. Provides a way to quickly download server binaries of a specified version. Provides dependency management and package build tools for library maintainers and gamemode writers alike."
	app.Version = version

	cli.VersionFlag = cli.BoolFlag{
		Name:  "appVersion, V",
		Usage: "sampctl version",
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return
	}
	err = os.MkdirAll(cacheDir, 0665)
	if err != nil {
		print.Erro("Failed to create cache directory at ", cacheDir, ": ", err)
		return
	}

	globalFlags := []cli.Flag{
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "output all detailed information - useful for debugging",
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
					Name:        "install",
					Usage:       "sampctl package install [package definition]",
					Description: "Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` downloads the contents.",
					Action:      packageInstall,
					Flags:       append(globalFlags, packageInstallFlags...),
				},
				{
					Name:        "build",
					Usage:       "sampctl package build",
					Description: "Builds a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:      packageBuild,
					Flags:       append(globalFlags, packageBuildFlags...),
				},
				{
					Name:        "run",
					Usage:       "sampctl package run",
					Description: "Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:      packageRun,
					Flags:       append(globalFlags, packageRunFlags...),
				},
			},
		},
		{
			Name:        "version",
			Description: "Show version number - this is also the version of the container image that will be used for `--container` runtimes.",
			Action:      cli.VersionPrinter,
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
		if c.GlobalBool("verbose") {
			print.SetVerbose()
		}
		if runtime.GOOS != "windows" {
			print.SetColoured()
		}
		return nil
	}

	err = app.Run(os.Args)
	if err != nil {
		print.Erro(err)
	}

	// quick and dirty stateless check to make sure update check doesn't run on *every* execution
	// instead, it will only check when the user happens to run the app during a minute and second
	// that are even numbers. 12:56:44 will work, 12:57:44 will not, etc...
	// this is done because the GitHub API has rate limits and we don't want to use all our requests
	// up on version checks when package management is more important.
	if time.Now().Minute()%2 == 0 && time.Now().Second()%2 == 0 {
		CheckForUpdates(app.Version)
	}
}

// CheckForUpdates uses the GitHub API to check if a new release is available.
func CheckForUpdates(thisVersion string) {
	client := github.NewClient(nil)
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)

	release, _, err := client.Repositories.GetLatestRelease(ctx, "Southclaws", "sampctl")
	if err != nil {
		print.Erro("Failed to check for latest sampctl release:", err)
	} else {
		latest, err := semver.NewVersion(release.GetTagName())
		if err != nil {
			print.Erro("Failed to interpret latest release tag as a semantic version:", err)
		}

		this, err := semver.NewVersion(thisVersion)
		if err != nil {
			print.Erro("Failed to interpret this version number as a semantic version:", err)
		}

		if latest.GreaterThan(this) {
			print.Info("\n-\n")
			print.Info(color.YellowString("sampctl version"), color.GreenString(latest.String()), color.YellowString("available!"))
			print.Info(color.YellowString("You are currently using"), color.GreenString(thisVersion))
			print.Info(color.YellowString("To upgrade, use the following command:"))
			switch runtime.GOOS {
			case "windows":
				print.Info(color.BlueString("  scoop update"))
				print.Info(color.BlueString("  scoop update sampctl"))
			case "linux":
				print.Info(color.YellowString("  Debian/Ubuntu based systems:"))
				print.Info(color.BlueString("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/install-deb.sh | sh"))
				print.Info(color.YellowString("  CentOS/Red Hat based systems"))
				print.Info(color.BlueString("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/install-rpm.sh | sh"))
			case "darwin":
				print.Info(color.BlueString("  brew update"))
				print.Info(color.BlueString("  brew upgrade sampctl"))
			}
			print.Info(color.YellowString("If you have any problems upgrading, please open an issue:"))
			print.Info(color.BlueString("  https://github.com/Southclaws/sampctl/issues/new"))
		}
	}
}
