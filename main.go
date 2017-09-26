package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	app := cli.NewApp()

	app.Author = "Southclaws"
	app.Email = "southclaws@gmail.com"
	app.Name = "sampctl"
	app.Version = "1.2.0-alpha.2"

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
		cli.BoolFlag{
			Name:  "container",
			Usage: "starts the server as a Linux container instead of running it in the current directory",
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
				dir := fullPath(c.String("dir"))
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
				dir := fullPath(c.String("dir"))
				container := c.Bool("container")

				var err error
				if container {
					err = RunContainer(endpoint, version, dir, app.Version)
				} else {
					err = Run(endpoint, version, dir)
				}

				return err
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
				dir := fullPath(c.String("dir"))
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

func fullPath(dir string) (path string) {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	if dir == "." {
		path = cwd
	} else {
		path = filepath.Join(cwd, dir)
	}

	return path
}
