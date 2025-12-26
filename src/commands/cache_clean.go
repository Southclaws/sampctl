package commands

import (
	"fmt"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
)

var cacheCleanFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "yes, y",
		Usage: "skip confirmation prompt",
	},
}

func CacheClean(c *cli.Context) error {
	applyVerboseFlag(c)

	if !c.Bool("yes") {
		print.Info("This will remove expired cached resources (older than 7 days)")
		print.Info("Continue? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" && response != "yes" {
			print.Info("Cache clean cancelled")
			return nil
		}
	}

	// Create resource manager
	gh := github.NewClient(nil)
	factory := resource.NewDefaultResourceFactory(gh)
	manager := resource.NewDefaultResourceManager(factory)

	print.Info("Cleaning expired cache entries...")

	err := manager.CleanCache()
	if err != nil {
		return errors.Wrap(err, "failed to clean cache")
	}

	print.Info("Cache cleaned successfully")
	return nil
}
