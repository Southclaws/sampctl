package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()

	app.Author = "Southclaws"
	app.Email = "southclaws@gmail.com"
	app.Name = "sampctl"
	app.Version = "1.1.1"

	cli.VersionFlag = cli.BoolFlag{
		Name:  "app-version, V",
		Usage: "show the app version number",
	}

	app.Flags = []cli.Flag{
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
	}

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "initialise a sa-mp server folder with a few questions, uses the cwd if --dir is not set",
			Action: func(c *cli.Context) error {
				endpoint := c.String("endpoint")
				version := c.String("version")
				dir := c.String("dir")

				err := InitialiseServer(version, dir)
				if err != nil {
					return errors.Wrap(err, "failed to initialise server")
				}

				err = GetPackage(endpoint, version, dir)
				if err != nil {
					return errors.Wrap(err, "failed to get package")
				}

				return nil
			},
			Flags: app.Flags,
		},
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run a sa-mp server, uses the cwd if --dir is not set",
			Action: func(c *cli.Context) error {
				endpoint := c.String("endpoint")
				version := c.String("version")
				dir := c.String("dir")

				err := GetPackage(endpoint, version, dir)
				if err != nil {
					return errors.Wrap(err, "failed to get package")
				}

				return Execute(endpoint, version, dir)
			},
			Flags: app.Flags,
		},
		{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download a version of the server, uses latest if --version is not specified",
			Action: func(c *cli.Context) error {
				endpoint := c.String("endpoint")
				version := c.String("version")
				dir := c.String("dir")
				fmt.Printf("Downloading package %s from endpoint %s into %s\n", version, endpoint, dir)
				return GetPackage(endpoint, version, dir)
			},
			Flags: app.Flags,
		},
	}

	cacheDir, err := getCacheDir()
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
