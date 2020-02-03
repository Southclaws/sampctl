package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/urfave/cli/v2"
)

// GenerateDocs generates markdown documentation for the commands in app
func GenerateDocs(app *cli.App) (result string) {
	buffer := bytes.Buffer{}

	buffer.WriteString(fmt.Sprintf("# `%s`\n\n%s - %s\n\n", app.Name, app.Version, app.Authors[0]))

	if app.Description != "" {
		buffer.WriteString(app.Description)
		buffer.WriteString("\n\n")
	}

	buffer.WriteString(fmt.Sprintf("## Commands (%d)\n\n", len(app.Commands)))

	for _, command := range app.Commands {
		generateCommandDocs(app.Name, command, &buffer)
		buffer.WriteString("---\n\n")
	}

	if len(app.Flags) > 0 {
		buffer.WriteString("## Global Flags\n\n")
		for _, flag := range app.Flags {
			flagInfo := strings.Split(flag.String(), "\t")
			buffer.WriteString(fmt.Sprintf("- `%s`: %s\n", flagInfo[0], flagInfo[1]))
		}
		buffer.WriteString("\n\n")
	}
	return buffer.String()
}

//nolint:gas
func generateCommandDocs(prefix string, command *cli.Command, buffer *bytes.Buffer) {
	buffer.WriteString(fmt.Sprintf("### `%s %s`\n\n", prefix, command.Name))
	if command.Usage != "" {
		buffer.WriteString(fmt.Sprintf("Usage: `%s`\n\n", command.Usage))
	}
	if command.Description != "" {
		buffer.WriteString(fmt.Sprintf("%s\n\n", command.Description))
	}
	if len(command.Flags) > 0 {
		buffer.WriteString("#### Flags\n\n")
		for _, flag := range command.Flags {
			flagInfo := strings.Split(flag.String(), "\t")
			buffer.WriteString(fmt.Sprintf("- `%s`: %s\n", flagInfo[0], flagInfo[1]))
		}
		buffer.WriteString("\n")
	}
	if len(command.Subcommands) > 0 {
		buffer.WriteString(fmt.Sprintf("#### Subcommands (%d)\n\n", len(command.Subcommands)))
		for _, subcommand := range command.Subcommands {
			generateCommandDocs(fmt.Sprint(prefix, " ", command.Name), subcommand, buffer)
		}
		buffer.WriteString("\n")
	}
}
