package rook

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestValidateUserAndRepo(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateUser("Southclaws"))
	require.NoError(t, validateRepo("pawn-errors"))

	require.Error(t, validateUser("bad user"))
	require.Error(t, validateRepo("bad/repo"))
}

func TestDoTemplate(t *testing.T) {
	t.Parallel()

	output, err := doTemplate("Hello {{.Repo}} {{.RepoEscaped}}", Answers{Repo: "my-repo"})
	require.NoError(t, err)
	assert.Equal(t, "Hello my-repo my--repo", output)

	output, err = doTemplate("{{", Answers{})
	require.Error(t, err)
	assert.Equal(t, "{{", output)
}

func TestGetTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()
	answers := Answers{Repo: "my-repo", User: "testuser"}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "raw.githubusercontent.com", req.URL.Host)
		assert.True(t, strings.HasSuffix(req.URL.Path, "/README.md"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("repo={{.Repo}} escaped={{.RepoEscaped}}")),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	require.NoError(t, getTemplateFile(tmpDir, "README.md", answers))
	contents, err := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "repo=my-repo escaped=my--repo", string(contents))
}

func TestGetTemplateFileDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	answers := Answers{Repo: "pkg"}
	original := filepath.Join(tmpDir, "README.md")
	require.NoError(t, os.WriteFile(original, []byte("existing"), 0o644))

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("new-content")),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	require.NoError(t, getTemplateFile(tmpDir, "README.md", answers))
	assert.FileExists(t, filepath.Join(tmpDir, "README.md-duplicate"))
	contents, err := os.ReadFile(filepath.Join(tmpDir, "README.md-duplicate"))
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(contents))
}
