package main

import (
	"os"

	"github.com/Southclaws/sampctl/commands"
	"github.com/Southclaws/sampctl/print"
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
