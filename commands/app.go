package commands

import (
	"context"
	"fmt"
	"io/ioutil"
	netHTTP "net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
)

var (
	gh          *github.Client
	gitAuth     transport.AuthMethod
	globalFlags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "output all detailed information - useful for debugging",
		},
		&cli.StringFlag{
			Name:  "platform",
			Value: "",
			Usage: "manually specify the target platform for downloaded binaries to either `windows`, `linux` or `darwin`.",
		},
		&cli.BoolFlag{
			Name:  "bare",
			Usage: "skip all pre-run configuration",
		},
	}
)

func Run(version string) error {
	app := cli.NewApp()

	app.Authors = []*cli.Author{
		{
			Name:  "Southclaws",
			Email: "hello@southcla.ws",
		},
	}
	app.Name = "sampctl"
	app.Usage = "The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer."
	app.Version = version
	app.EnableBashCompletion = true

	cli.VersionFlag = &cli.BoolFlag{
		Name:  "appVersion, V",
		Usage: "sampctl version",
	}

	//nolint:lll
	app.Commands = []*cli.Command{
		{
			Name:        "server",
			Aliases:     []string{"s"},
			Usage:       "sampctl server <subcommand>",
			Description: "For managing servers and runtime configurations.",
			Subcommands: []*cli.Command{
				{
					Name:        "init",
					Usage:       "sampctl server init",
					Description: "Bootstrap a new SA:MP server and generates a `samp.json`/`samp.yaml` configuration based on user input. If `gamemodes`, `filterscripts` or `plugins` directories are present, you will be prompted to select relevant files.",
					Action:      ServerInit,
					Flags:       append(globalFlags, ServerInitFlags...),
				},
				{
					Name:        "download",
					Usage:       "sampctl server download",
					Description: "Downloads the files necessary to run a SA:MP server to the current directory (unless `--dir` specified). Will download the latest stable (non RC) server version unless `--version` is specified.",
					Action:      ServerDownload,
					Flags:       append(globalFlags, ServerDownloadFlags...),
				},
				{
					Name:        "ensure",
					Usage:       "sampctl server ensure",
					Description: "Ensures the server environment is representative of the configuration specified in `samp.json`/`samp.yaml` - downloads server binaries and plugin files if necessary and generates a `server.cfg` file.",
					Action:      ServerEnsure,
					Flags:       append(globalFlags, ServerEnsureFlags...),
				},
				{
					Name:        "run",
					Usage:       "sampctl server run",
					Description: "Generates a `server.cfg` file based on the configuration inside `samp.json`/`samp.yaml` then executes the server process and automatically restarts it on crashes.",
					Action:      ServerRun,
					Flags:       append(globalFlags, ServerRunFlags...),
				},
			},
		},
		{
			Name:        "package",
			Aliases:     []string{"p"},
			Usage:       "sampctl package <subcommand>",
			Description: "For managing Pawn packages such as gamemodes and libraries.",
			Subcommands: []*cli.Command{
				PackageInit,
				{
					Name:        "ensure",
					Usage:       "sampctl package ensure",
					Description: "Ensures dependencies are up to date based on the `dependencies` field in `pawn.json`/`pawn.yaml`.",
					Action:      PackageEnsure,
					Flags:       append(globalFlags, PackageEnsureFlags...),
				},
				{
					Name:         "install",
					Usage:        "sampctl package install [package definition]",
					Description:  "Installs a new package by adding it to the `dependencies` field in `pawn.json`/`pawn.yaml` and downloads the contents.",
					Action:       PackageInstall,
					Flags:        append(globalFlags, PackageInstallFlags...),
					BashComplete: PackageInstallBash,
				},
				{
					Name:        "uninstall",
					Usage:       "sampctl package uninstall [package definition]",
					Description: "Uninstalls package by removing it from the `dependencies` field in `pawn.json`/`pawn.yaml` and deletes the contents.",
					Action:      PackageUninstall,
					Flags:       append(globalFlags, PackageUninstallFlags...),
					// BashComplete: PackageUninstallBash,
				},
				{
					Name:        "release",
					Usage:       "sampctl package release",
					Description: "Creates a release version and tags the repository with the next version number, creates a GitHub release with archived package files.",
					Action:      PackageRelease,
					Flags:       append(globalFlags, PackageReleaseFlags...),
				},
				{
					Name:         "get",
					Usage:        "sampctl package get [package definition] (target path)",
					Description:  "Clones a GitHub package to either a directory named after the repo or, if the cwd is empty, the cwd and then ensures the package.",
					Action:       PackageGet,
					Flags:        append(globalFlags, PackageGetFlags...),
					BashComplete: PackageGetBash,
				},
				{
					Name:         "build",
					Usage:        "sampctl package build [build name]",
					Description:  "Builds a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:       PackageBuild,
					Flags:        append(globalFlags, PackageBuildFlags...),
					BashComplete: PackageBuildBash,
				},
				{
					Name:        "run",
					Usage:       "sampctl package run",
					Description: "Compiles and runs a package defined by a `pawn.json`/`pawn.yaml` file.",
					Action:      PackageRun,
					Flags:       append(globalFlags, PackageRunFlags...),
				},
				{
					Name:        "template",
					Usage:       "sampctl package template <subcommand>",
					Description: "Provides commands for package templates",
					Subcommands: []*cli.Command{
						{
							Name:        "make",
							Usage:       "sampctl package template make [name]",
							Description: "Creates a template package from the current directory if it is a package.",
							Action:      PackageTemplateMake,
							Flags:       append(globalFlags, PackageTemplateMakeFlags...),
						},
						{
							Name:        "build",
							Usage:       "sampctl package template build [template] [filename]",
							Description: "Builds the specified file in the context of the given template.",
							Action:      PackageTemplateBuild,
							Flags:       append(globalFlags, PackageTemplateBuildFlags...),
						},
						{
							Name:        "run",
							Usage:       "sampctl package template run [template] [filename]",
							Description: "Builds and runs the specified file in the context of the given template.",
							Action:      PackageTemplateRun,
							Flags:       append(globalFlags, PackageTemplateRunFlags...),
						},
					},
				},
			},
		},
		{
			Name:        "version",
			Description: "Show version number - this is also the version of the container image that will be used for `--container` runtimes.",
			Action: func(c *cli.Context) error {
				cli.VersionPrinter(c)
				return nil
			},
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
		err := godotenv.Load(".env")
		if err != nil {
			print.Verb(err)
		}

		verbose := c.Bool("verbose")

		// "bare" mode is for CI use only
		if c.Bool("bare") {
			return nil
		}

		if verbose {
			print.SetVerbose()
			print.Verb("Verbose logging active")
		}
		if runtime.GOOS != "windows" {
			print.SetColoured()
		}

		return nil
	}
	app.After = func(c *cli.Context) error {
		if c.IsSet("generate-bash-completion") {
			return nil
		}

		// quick and dirty stateless check to make sure update check doesn't run on *every* execution
		// instead, it will only check when the user happens to run the app during a minute and second
		// that are even numbers. 12:56:44 will work, 12:57:44 will not, etc...
		// this is done because the GitHub API has rate limits and we don't want to use all our requests
		// up on version checks when package management is more important.
		if !c.IsSet("generate-bash-completion") &&
			!c.IsSet("bare") &&
			time.Now().Minute()%2 == 0 &&
			time.Now().Second()%2 == 0 {
			CheckForUpdates(gh, version)
		}
		return nil
	}

	return app.Run(os.Args)
}

func GitHubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	} else {
		return github.NewClient(
			oauth2.NewClient(context.Background(),
				oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})),
		)
	}
}

func GitAuth(config types.Config) transport.AuthMethod {
	if config.GitUsername != "" && config.GitPassword != "" {
		return http.NewBasicAuth(config.GitUsername, config.GitPassword)
	} else {
		a, err := ssh.DefaultAuthBuilder("git")
		if err != nil {
			print.Verb("Failed to set up SSH:", err)
			return nil
		}
		return a
	}
}

// CheckForUpdates uses the GitHub API to check if a new release is available.
func CheckForUpdates(gh *github.Client, thisVersion string) {
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

func autoComplete(c *cli.Context) (err error) {
	var flavour = "bash"
	if c.String("flavour") == "zsh" {
		flavour = "zsh"
	}

	resp, err := netHTTP.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/urfave/cli/master/autocomplete/%s_autocomplete",
		flavour,
	))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.Errorf("failed to get bash completion: %s", resp.Status)
	}

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return
	}

	completionFile := filepath.Join(cacheDir, "autocomplete")

	err = ioutil.WriteFile(completionFile, contents, 0700)
	if err != nil {
		return
	}

	print.Info("Successfully written", flavour, "completion to", completionFile)
	print.Info("To enable, add the following line to your .bashrc file (or equivalent)")
	print.Info("PROG=sampctl source", completionFile)

	return nil
}
