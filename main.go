package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
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
				dir := fullPath(c.String("dir"))
				endpoint := c.String("endpoint")
				err := InitialiseServer(version, dir)
				if err != nil {
					return errors.Wrap(err, "failed to initialise server")
				}

				err = GetServerPackage(endpoint, version, dir, false)
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
				dir := fullPath(c.String("dir"))
				endpoint := c.String("endpoint")
				container := c.Bool("container")

				var err error
				errs := ValidateServerDir(dir, version)
				if errs != nil {
					fmt.Println(errs)
					err = GetServerPackage(endpoint, version, dir, false)
					if err != nil {
						return errors.Wrap(err, "failed to get server package")
					}
				}

				if container {
					err = RunContainer(endpoint, version, dir, app.Version)
				} else {
					err = Run(endpoint, version, dir)
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
				dir := fullPath(c.String("dir"))
				endpoint := c.String("endpoint")
				return GetServerPackage(endpoint, version, dir, false)
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
			Name:    "exec",
			Aliases: []string{"e"},
			Usage:   "execute an amx file as a SA:MP gamemode for quick testing in a temporary server installation",
			Action: func(c *cli.Context) error {
				if c.NArg() != 1 {
					return errors.New("argument required: file to execute")
				}

				file := c.Args().First()
				version := c.String("version")
				container := c.Bool("container")
				endpoint := c.String("endpoint")
				cacheDir, err := GetCacheDir()
				if err != nil {
					return err
				}
				dir := filepath.Join(cacheDir, "runtime", version)

				filePath := fullPath(file)

				err = PrepareRuntime(endpoint, version, dir)
				if err != nil {
					return err
				}

				err = CopyFileToRuntime(cacheDir, version, filePath)
				if err != nil {
					return err
				}

				if container {
					err = RunContainer(endpoint, version, dir, app.Version)
				} else {
					err = Run(endpoint, version, dir)
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
			Name:    "compile",
			Aliases: []string{"c"},
			Usage:   "compile a .pwn file to an .amx file in the same directory",
			Action: func(c *cli.Context) error {
				if c.NArg() != 1 {
					return errors.New("argument required: file to compile")
				}

				pawnccVersion := c.String("pawncc-version")
				inputFile := fullPath(c.Args().First())
				outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".amx"

				if !exists(inputFile) {
					return errors.Errorf("source file '%s' does not exist", inputFile)
				}

				cacheDir, err := GetCacheDir()
				if err != nil {
					return err
				}

				err = CompileSource(inputFile, outputFile, cacheDir, pawnccVersion)

				return err
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "pawncc-version",
					Value: "3.10.2",
					Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
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

	cacheDir, err := GetCacheDir()
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
	path, err := filepath.Abs(dir)
	if err != nil {
		panic(err)
	}

	return path
}
