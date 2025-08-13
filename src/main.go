package main

import (
	"os"

	"github.com/Southclaws/sampctl/src/commands"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

var (
	version = "master"
)

func main() {
	if err := commands.Run(os.Args, version); err != nil {
		print.Erro(err)
		os.Exit(1)
	}
}
