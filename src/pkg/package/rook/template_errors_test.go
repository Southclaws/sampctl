package rook

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateRenderingErrorsPropagate(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		message string
	}{
		{name: "parse", content: "{{", message: "failed to parse input as template"},
		{name: "execute", content: "{{.MissingField}}", message: "failed to execute template"},
	} {
		t.Run(test.name, func(t *testing.T) {
			answers := Answers{Repo: "my-repo", PublishMode: publishGitHub}
			_, renderErr := doTemplate(test.content, answers)
			require.ErrorContains(t, renderErr, test.message)

			oldTransport := http.DefaultTransport
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				content := "repo={{.Repo}}"
				if strings.HasSuffix(req.URL.Path, "/README.md") {
					content = test.content
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(content))}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = oldTransport })

			t.Run("file", func(t *testing.T) {
				dir := t.TempDir()
				err := getTemplateFile(t.Context(), dir, "README.md", answers)
				assert.NoFileExists(t, filepath.Join(dir, "README.md"))
				require.EqualError(t, err, renderErr.Error())
			})

			t.Run("initialisation", func(t *testing.T) {
				dir := t.TempDir()
				err := fetchInitTemplates(t.Context(), dir, starterProfile{Readme: true, EditorConfig: true}, answers)
				assert.NoFileExists(t, filepath.Join(dir, "README.md"))
				content, readErr := os.ReadFile(filepath.Join(dir, ".editorconfig"))
				require.NoError(t, readErr)
				assert.Equal(t, "repo=my-repo", string(content))
				require.ErrorContains(t, err, "failed to fetch template files: README.md")
				assert.ErrorContains(t, err, test.message)
				assert.EqualError(t, errors.Cause(err), errors.Cause(renderErr).Error())
				if test.name == "execute" {
					var executionError template.ExecError
					require.ErrorAs(t, err, &executionError)
				}
			})
		})
	}
}
