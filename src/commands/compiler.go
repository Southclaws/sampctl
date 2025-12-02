package commands

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

var compilerFlags = []cli.Flag{
	//
}

func compilerList(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	presets := build.GetPredefinedCompilers()

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Preset", "Description", "Repository"})

	for name, preset := range presets {
		description := preset.Description
		if description == "" {
			description = "Standard " + name + " compiler"
		}

		repo := preset.Site + "/" + preset.User + "/" + preset.Repo
		if repo == "" {
			repo = "N/A"
		}

		t.AppendRows([]table.Row{
			{name, description, repo},
		})
	}

	t.Render()

	print.Info("")
	print.Info("Usage: Set the 'preset' field in your build configuration to use a different compiler:")
	print.Info(`  "compiler": {`)
	print.Info(`    "preset": "openmp"`)
	print.Info(`  }`)
	print.Info("")
	print.Info("Or override specific settings:")
	print.Info(`  "compiler": {`)
	print.Info(`    "preset": "pawn-lang",`)
	print.Info(`    "version": "3.10.10"`)
	print.Info(`  }`)

	return nil
}
