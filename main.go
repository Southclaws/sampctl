package main

import (
	_ "github.com/joho/godotenv/autoload"

	"github.com/Southclaws/sampctl/commands"
	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
)

var (
	version = "master"
	config  *types.Config // global config
)

func main() {
	cacheDir, err := download.GetCacheDir()
	if err != nil {
		print.Erro("Failed to retrieve cache directory path (attempted <user folder>/.samp) ", err)
		return
	}

	if err = commands.Run(version); err != nil {
		print.Erro(err)
	}

	if config != nil {
		err = types.WriteConfig(cacheDir, *config)
		if err != nil {
			print.Erro("Failed to write updated configuration file to", cacheDir, "-", err)
			return
		}
	}
}
