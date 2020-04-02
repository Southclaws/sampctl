package main

import (
	"os"

	"github.com/Southclaws/sampctl/commands"
	"github.com/Southclaws/sampctl/print"
)

func main() {
	if err := commands.Run(os.Args); err != nil {
		print.Erro(err)
		os.Exit(1)
	}
}
