//go:build linux || darwin

package rook

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptAnswersPreserveValidationAndDefaults(t *testing.T) {
	promptLines := func(lines ...string) string {
		var keys strings.Builder
		for _, line := range lines {
			// Supply terminal size and cursor reports requested by line prompts.
			keys.WriteString("\x1b[24;80R\x1b[1;1R")
			keys.WriteString(line)
			keys.WriteByte('\r')
		}
		return keys.String()
	}

	tests := []struct {
		name       string
		keys       string
		repository string
		generate   bool
		github     bool
	}{
		{name: "defaults", keys: promptLines("", "", ""), repository: "fixture/default", generate: true},
		{name: "validated explicit answers", keys: promptLines("invalid", "fixture/project", "n", "y"), repository: "fixture/project", github: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, writer, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, input.Close()) })

			_, writeErr := io.WriteString(writer, test.keys)
			closeErr := writer.Close()
			require.NoError(t, writeErr)
			require.NoError(t, closeErr)

			output, err := os.CreateTemp(t.TempDir(), "prompt-output-")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, output.Close()) })

			questions := []*survey.Question{
				{
					Name:     "Repository",
					Prompt:   &survey.Input{Message: "Repository", Default: "fixture/default"},
					Validate: validateRepositorySpec,
				},
				{Name: "Generate", Prompt: &survey.Confirm{Message: "Generate", Default: true}},
				{Name: "GitHub", Prompt: &survey.Confirm{Message: "GitHub", Default: false}},
			}
			var answers struct {
				Repository string
				Generate   bool
				GitHub     bool
			}
			err = survey.Ask(questions, &answers, survey.WithStdio(selectPromptInput{File: input}, output, output))
			require.NoError(t, err)
			assert.Equal(t, test.repository, answers.Repository)
			assert.Equal(t, test.generate, answers.Generate)
			assert.Equal(t, test.github, answers.GitHub)
		})
	}
}
