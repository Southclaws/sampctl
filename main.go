package main

import (
	"fmt"
	"os"

	"gopkg.in/urfave/cli.v1"
)

func main() {
	// load settings from json, env or cmd (cobra/viper?)
	// generate server.cfg in working dir
	// launch server as child process
	// monitor child process for non-zero exits
	// pipe stdout to custom log handler

	app := cli.NewApp()

	app.Version = "0.1.0"
	app.Author = "Southclaws"
	app.Email = "southclaws@gmail.com"
	app.Name = "sampctl"

	fmt.Println("sampctl")

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "ver",
			Value: "samp037svr_R2-2-1",
			Usage: "server version - corresponds to http://files.sa-mp.com packages without the .tar.gz",
		},
		cli.StringFlag{
			Name:  "dir",
			Value: ".",
			Usage: "working directory for the server - by default, uses the current directory",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "run",
			Aliases: []string{"r"},
			Usage:   "run a sa-mp server, uses the cwd if --dir is not set",
			Action: func(c *cli.Context) error {
				// check if sa-mp server present in current directory or --dir path
				// if not, run the "download" route first
				// then run the server as normal
				return nil
			},
		},
		{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download a version of the server, uses latest if --ver is not specified",
			Action: func(c *cli.Context) error {
				// --ver refers to a filename on http://files.sa-mp.com without the extension
				// samp037svr_R2-2-1 maps to http://files.sa-mp.com/samp037svr_R2-2-1.tar.gz etc
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("Exited with error: %v\n", err)
	}
}
