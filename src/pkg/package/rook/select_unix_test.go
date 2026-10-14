//go:build linux || darwin

package rook

import (
	"io"
	"os"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectHandlesUnmatchedFilter(t *testing.T) {
	options := initModeOptions(nil, nil)
	tests := []struct {
		name           string
		keys           string
		expected       string
		withoutDefault bool
		wantInterrupt  bool
	}{
		{
			name:     "navigation and clearing restores selection",
			keys:     "\\\x1b[A\x1b[B\r\x18\x1b[B\r",
			expected: options[1],
		},
		{
			name:     "ordinary navigation selects next option",
			keys:     "\x1b[B\r",
			expected: options[1],
		},
		{
			name:     "backspace restores matching filter",
			keys:     "libraryx\x7f\r",
			expected: options[1],
		},
		{
			name:     "end of transmission uses default",
			keys:     "\\\x04",
			expected: options[0],
		},
		{
			name:          "interrupt returns an error",
			keys:          "\\\x03",
			wantInterrupt: true,
		},
		{
			name:           "end of transmission without default retries required selection",
			keys:           "\\\x04\r",
			expected:       options[0],
			withoutDefault: true,
		},
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

			output, err := os.CreateTemp(t.TempDir(), "select-output-")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, output.Close()) })

			prompt := &survey.Select{
				Message: "How do you want to start this package?",
				Options: options,
				Default: options[0],
			}
			var reader terminal.FileReader = input
			if test.withoutDefault {
				prompt.Default = nil
				reader = selectPromptInput{File: input}
			}
			var answer string
			err = survey.AskOne(prompt, &answer, survey.WithValidator(requiredSelection), survey.WithStdio(reader, output, output))
			if test.wantInterrupt {
				require.ErrorIs(t, err, terminal.InterruptErr)
				assert.Empty(t, answer)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.expected, answer)
			if test.withoutDefault {
				contents, err := os.ReadFile(output.Name())
				require.NoError(t, err)
				assert.Contains(t, string(contents), "Value is required")
			}
		})
	}
}

type selectPromptInput struct {
	*os.File
}

func (input selectPromptInput) Read(data []byte) (int, error) {
	// Keep retry input out of the reader discarded after an invalid answer.
	return input.File.Read(data[:min(len(data), 1)])
}
