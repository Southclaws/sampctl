package commands

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
)

func completePackageList(c *cli.Context) {
	env, err := getCommandEnv(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to get config dir:", err)
		return
	}

	packages, err := download.GetPackageList(env.CacheDir)
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
