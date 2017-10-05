package main

import (
	"bytes"
	"fmt"

	"gopkg.in/urfave/cli.v1"
)

// GenerateDocs generates markdown documentation for the commands in app
func GenerateDocs(app *cli.App) (result string) {
	buffer := bytes.Buffer{}

	buffer.WriteString(fmt.Sprintf("# `%s`\n%s\n\n", app.Name, app.Version))

	for _, command := range app.Commands {
		buffer.WriteString(fmt.Sprintf("## `%s`\n\n", command.Name))
		if command.Usage != "" {
			buffer.WriteString(command.Usage)
			buffer.WriteString("\n\n")
		}
		if command.Description != "" {
			buffer.WriteString(command.Description)
			buffer.WriteString("\n\n")
		}
		if len(command.Flags) > 0 {
			buffer.WriteString("### Flags\n\n")
			for _, flag := range command.Flags {
				buffer.WriteString(fmt.Sprintf("- %s\n", flag.GetName()))
			}
			buffer.WriteString("\n\n")
		}
	}
	return buffer.String()
}
