package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/rook"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

var packageGetFlags = []cli.Flag{}

func packageGet(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	if len(c.Args()) == 0 {
		cli.ShowCommandHelpAndExit(c, "get", 0)
		return nil
	}

	cacheDir := download.GetCacheDir()

	dep, err := versioning.DependencyString(c.Args().First()).Explode()
	if err != nil {
		return err
	}

	dir := c.Args().Get(1)
	if dir == "" {
		dir = util.FullPath(".")
	}

	err = rook.Get(context.Background(), gh, dep, dir, gitAuth, platform(c), cacheDir)
	if err != nil {
		return err
	}

	print.Info("successfully cloned package")

	return nil
}

func packageGetBash(c *cli.Context) {
	cacheDir := download.GetCacheDir()

	packages, err := download.GetPackageList(cacheDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to get package list:", err)
		return
	}

	query := c.Args().First()
	for _, pkg := range packages {
		if strings.HasPrefix(pkg.String(), query) {
			fmt.Println(pkg)
		}
	}
}
