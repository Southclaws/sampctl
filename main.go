package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/server"
	"github.com/Southclaws/sampctl/settings"
	"github.com/Southclaws/sampctl/util"
)

var version = "master"

func main() {
	app := cli.NewApp()

	app.Author = "Southclaws"
	app.Email = "southclaws@gmail.com"
	app.Name = "sampctl"
	app.Description = "A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency."
	app.Version = version

	cli.VersionFlag = cli.BoolFlag{
		Name:  "app-version, V",
		Usage: "show the app version number",
	}

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "initialise a sa-mp server folder with a few questions, uses the cwd if --dir is not set",
			Action: func(c *cli.Context) error {
				version := c.String("version")
				dir := util.FullPath(c.String("dir"))
				endpoint := c.String("endpoint")
				err := settings.InitialiseServer(version, dir)
				if err != nil {
					return errors.Wrap(err, "failed to initialise server")
				}

				err = server.GetServerPackage(endpoint, version, dir)
				if err != nil {
					return errors.Wrap(err, "failed to get package")
				}

				return nil
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "version",
					Value: "0.3.7",
					Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
				},
				cli.StringFlag{
					Name:  "dir",
					Value: ".",
					Usage: "working directory for the server - by default, uses the current directory",
				},
				cli.StringFlag{
					Name:  "endpoint",
					Value: "http://files.sa-mp.com",
					Usage: "endpoint to download packages from",
				},
			},
		},
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run a sa-mp server, uses the cwd if --dir is not set",
			Action: func(c *cli.Context) error {
				version := c.String("version")
				dir := util.FullPath(c.String("dir"))
				endpoint := c.String("endpoint")
				container := c.Bool("container")

				var err error
				errs := server.ValidateServerDir(dir, version)
				if errs != nil {
					fmt.Println(errs)
					err = server.GetServerPackage(endpoint, version, dir)
					if err != nil {
						return errors.Wrap(err, "failed to get server package")
					}
				}

				if container {
					err = server.RunContainer(endpoint, version, dir, app.Version)
				} else {
					err = server.Run(endpoint, version, dir)
				}

				return err
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "version",
					Value: "0.3.7",
					Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
				},
				cli.StringFlag{
					Name:  "dir",
					Value: ".",
					Usage: "working directory for the server - by default, uses the current directory",
				},
				cli.StringFlag{
					Name:  "endpoint",
					Value: "http://files.sa-mp.com",
					Usage: "endpoint to download packages from",
				},
				cli.BoolFlag{
					Name:  "container",
					Usage: "starts the server as a Linux container instead of running it in the current directory",
				},
			},
		},
		{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download a version of the server, uses latest if --version is not specified",
			Action: func(c *cli.Context) error {
				version := c.String("version")
				dir := util.FullPath(c.String("dir"))
				endpoint := c.String("endpoint")
				return server.GetServerPackage(endpoint, version, dir)
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "version",
					Value: "0.3.7",
					Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
				},
				cli.StringFlag{
					Name:  "dir",
					Value: ".",
					Usage: "working directory for the server - by default, uses the current directory",
				},
				cli.StringFlag{
					Name:  "endpoint",
					Value: "http://files.sa-mp.com",
					Usage: "endpoint to download packages from",
				},
			},
		},
		{
			Name:    "project",
			Aliases: []string{"p"},
			Usage:   "project level commands for managing packages and gamemodes",
			Subcommands: []cli.Command{
				{
					Name:    "run",
					Aliases: []string{"r"},
					Usage:   "compiles and runs a project defined by a pawn.json or pawn.yaml file",
					Action: func(c *cli.Context) error {
						// version := c.String("version")
						// container := c.Bool("container")
						// endpoint := c.String("endpoint")
						// cacheDir, err := download.GetCacheDir()
						// if err != nil {
						// 	return err
						// }
						// dir := filepath.Join(cacheDir, "runtime", version)

						// todo: compile as a build job then copy resultant .amx to runtime
						//
						// filePath := util.FullPath(file)

						// err = server.PrepareRuntime(endpoint, version, dir)
						// if err != nil {
						// 	return err
						// }

						// err = server.CopyFileToRuntime(cacheDir, version, filePath)
						// if err != nil {
						// 	return err
						// }

						// if container {
						// 	err = server.RunContainer(endpoint, version, dir, app.Version)
						// } else {
						// 	err = server.Run(endpoint, version, dir)
						// }

						return nil
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "version",
							Value: "0.3.7",
							Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
						},
						cli.StringFlag{
							Name:  "endpoint",
							Value: "http://files.sa-mp.com",
							Usage: "endpoint to download packages from",
						},
						cli.BoolFlag{
							Name:  "container",
							Usage: "starts the server as a Linux container instead of running it in the current directory",
						},
					},
				},
				{
					Name:    "build",
					Aliases: []string{"c"},
					Usage:   "builds a project defined by a pawn.json or pawn.yaml file",
					Action: func(c *cli.Context) error {
						// version := c.String("version")
						// inputFile := util.FullPath(c.Args().First())
						// outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".amx"

						// if !util.Exists(inputFile) {
						// 	return errors.Errorf("source file '%s' does not exist", inputFile)
						// }

						// cacheDir, err := download.GetCacheDir()
						// if err != nil {
						// 	return err
						// }

						// err = rook.BuildProject(inputFile, outputFile, cacheDir, version, includes)

						return nil
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "pawncc-version",
							Value: "3.10.2",
							Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
						},
					},
				},
			},
		},
		{
			Name:  "docgen",
			Usage: "generate documentation - mainly just for CI usage, the readme file will always be up to date.",
			Action: func(c *cli.Context) error {
				docs := GenerateDocs(c.App)
				fmt.Print(docs)
				return nil
			},
		},
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

	err = app.Run(os.Args)
	if err != nil {
		fmt.Printf("Exited with error: %v\n", err)
	}
}
