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
		fmt.Println("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return
	}
	err = os.MkdirAll(cacheDir, 0665)
	if err != nil {
		fmt.Println("Failed to create cache directory at ", cacheDir, ": ", err)
		return
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
					Flags:       serverInitFlags,
				},
				{
					Name:        "download",
					Usage:       "sampctl server download",
					Description: "Downloads the files necessary to run a SA:MP server to the current directory (unless `--dir` specified). Will download the latest stable (non RC) server version unless `--version` is specified.",
					Action:      serverDownload,
					Flags:       serverDownloadFlags,
				},
				{
					Name:        "ensure",
					Usage:       "sampctl server ensure",
					Description: "Ensures the server environment is representative of the configuration specified in `samp.json`/`samp.yaml` - downloads server binaries and plugin files if necessary and generates a `server.cfg` file.",
					Action:      serverEnsure,
					Flags:       serverEnsureFlags,
				},
				{
					Name:        "run",
					Usage:       "sampctl server run",
					Description: "Generates a `server.cfg` file based on the configuration inside `samp.json`/`samp.yaml` then executes the server process and automatically restarts it on crashes.",
					Action:      serverRun,
					Flags:       serverRunFlags,
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
					Flags:       packageInitFlags,
				},
				{
					Name:        "ensure",
					Usage:       "sampctl package ensure",
					Description: "Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`/`pawn.yaml`.",
					Action:      packageEnsure,
					Flags:       packageEnsureFlags,
				},
				{
					Name:        "install",
					Usage:       "sampctl package install [package definition]",
					Description: "Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` downloads the contents.",
					Action:      packageInstall,
					Flags:       packageInstallFlags,
				},
				{
					Name:        "build",
					Usage:       "sampctl package build",
					Description: "Builds a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:      packageBuild,
					Flags:       packageBuildFlags,
				},
				{
					Name:        "run",
					Usage:       "sampctl package run",
					Description: "Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:      packageRun,
					Flags:       packageRunFlags,
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

	err = app.Run(os.Args)
	if err != nil {
		fmt.Printf("Exited with error: %v\n", err)
	}

	client := github.NewClient(nil)
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)

	release, _, err := client.Repositories.GetLatestRelease(ctx, "Southclaws", "sampctl")
	if err != nil {
		fmt.Println(color.RedString("Failed to check for latest sampctl release:"), err)
	} else {
		latest, err := semver.NewVersion(release.GetTagName())
		if err != nil {
			fmt.Println(color.RedString("Failed to interpret latest release tag as a semantic version:"), err)
		}

		this, err := semver.NewVersion(app.Version)
		if err != nil {
			fmt.Println(color.RedString("Failed to interpret this version number as a semantic version:"), err)
		}

		if latest.GreaterThan(this) {
			fmt.Println("\n-\n")
			fmt.Println(color.YellowString("sampctl version"), color.GreenString(latest.String()), color.YellowString("available!"))
			fmt.Println(color.YellowString("To upgrade, use the following command:"))
			switch runtime.GOOS {
			case "windows":
				fmt.Println(color.BlueString("  scoop update"))
				fmt.Println(color.BlueString("  scoop update sampctl"))
			case "linux":
				fmt.Println(color.YellowString("  Debian/Ubuntu based systems:"))
				fmt.Println(color.BlueString("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/install-deb.sh | sh"))
				fmt.Println(color.YellowString("  CentOS/Red Hat based systems"))
				fmt.Println(color.BlueString("  curl https://raw.githubusercontent.com/Southclaws/sampctl/master/install-rpm.sh | sh"))
			case "darwin":
				fmt.Println(color.BlueString("  brew update"))
				fmt.Println(color.BlueString("  brew upgrade sampctl"))
			}
			fmt.Println(color.YellowString("If you have any problems upgrading, please open an issue:"))
			fmt.Println(color.BlueString("  https://github.com/Southclaws/sampctl/issues/new"))
		}
	}
}
