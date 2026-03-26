package rook

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
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

func TestRepositorySpecHelpers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "configured-user", defaultMetadataUser("configured-user"))
	assert.NotEmpty(t, defaultMetadataUser(""))

	user, repo, err := splitRepositorySpec("Southclaws/sampctl")
	require.NoError(t, err)
	assert.Equal(t, "Southclaws", user)
	assert.Equal(t, "sampctl", repo)

	assert.Equal(t, "your-github-user/my-package", defaultRepositorySpec("", "my package"))
	assert.Equal(t, "Southclaws/my-package", defaultRepositorySpec("Southclaws", "my package"))

	require.Error(t, validateRepositorySpec("invalid"))
	require.Error(t, validateRepositorySpec("bad user/repo"))
}

func TestInitModeOptions(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{initModeUsePwn, initModeNewGamemode, initModeNewHarness},
		initModeOptions([]string{"main.pwn"}, []string{"lib.inc"}),
	)
	assert.Equal(t,
		[]string{initModeUseInc, initModeNewGamemode, initModeNewHarness},
		initModeOptions(nil, []string{"lib.inc"}),
	)
	assert.Equal(t,
		[]string{initModeNewGamemode, initModeNewHarness},
		initModeOptions(nil, nil),
	)
}

func TestStarterProfileFor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, starterProfile{}, starterProfileFor(starterMinimal))
	assert.Equal(t, starterProfile{
		GitIgnore:    true,
		Readme:       true,
		Git:          true,
		EditorConfig: true,
	}, starterProfileFor(starterStandard))
	assert.Equal(t, "vscode", starterProfileFor(starterVSCode).Editor)
	assert.Equal(t, "sublime", starterProfileFor(starterSublime).Editor)
}

func TestRuntimeChoiceHelpers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, runtimeOptionOpenMP, runtimeChoiceFromPreset(""))
	assert.Equal(t, runtimeOptionOpenMP, runtimeChoiceFromPreset("openmp"))
	assert.Equal(t, runtimeOptionSAMP, runtimeChoiceFromPreset("samp"))
	assert.Equal(t, "openmp", presetFromRuntimeChoice(runtimeOptionOpenMP))
	assert.Equal(t, "samp", presetFromRuntimeChoice(runtimeOptionSAMP))
}

func TestFormatChoiceHelpers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, formatOptionJSON, formatChoiceFromFormat(""))
	assert.Equal(t, formatOptionJSON, formatChoiceFromFormat("json"))
	assert.Equal(t, formatOptionYAML, formatChoiceFromFormat("yaml"))
	assert.Equal(t, "json", formatFromChoice(formatOptionJSON))
	assert.Equal(t, "yaml", formatFromChoice(formatOptionYAML))
}

func TestPublishChoiceHelpers(t *testing.T) {
	t.Parallel()

	assert.Equal(t, publishLocal, publishChoiceFromMode(""))
	assert.Equal(t, publishLocal, publishChoiceFromMode(publishLocal))
	assert.Equal(t, publishGitHub, publishChoiceFromMode(publishGitHub))
	assert.Equal(t, publishLocal, publishModeFromChoice(publishLocal))
	assert.Equal(t, publishGitHub, publishModeFromChoice(publishGitHub))
}

func TestReleaseHintForPublishMode(t *testing.T) {
	t.Parallel()

	assert.Empty(t, releaseHintForPublishMode(publishLocal))
	assert.Contains(t, releaseHintForPublishMode(publishGitHub), "sampctl release")
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
	answers := Answers{Repo: "my-repo", User: "testuser", PublishMode: publishGitHub}

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

	require.NoError(t, getTemplateFile(context.Background(), tmpDir, "README.md", answers))
	contents, err := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "repo=my-repo escaped=my--repo", string(contents))
}

func TestGetTemplateFileDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	answers := Answers{Repo: "pkg", PublishMode: publishGitHub}
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

	require.NoError(t, getTemplateFile(context.Background(), tmpDir, "README.md", answers))
	assert.FileExists(t, filepath.Join(tmpDir, "README.md-duplicate"))
	contents, err := os.ReadFile(filepath.Join(tmpDir, "README.md-duplicate"))
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(contents))
}

func TestGetTemplateFileHonorsContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	answers := Answers{Repo: "my-repo", PublishMode: publishGitHub}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	defer func() { http.DefaultTransport = oldTransport }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := getTemplateFile(ctx, tmpDir, "README.md", answers)
	require.ErrorIs(t, err, context.Canceled)
}

func TestGetTemplateFileUsesLocalReadmeTemplateForLocalPackages(t *testing.T) {
	tmpDir := t.TempDir()
	answers := Answers{Repo: "my-local-package", PublishMode: publishLocal}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request for local README: %s", req.URL.String())
		return nil, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	require.NoError(t, getTemplateFile(context.Background(), tmpDir, "README.md", answers))
	contents, err := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(contents), "# my-local-package")
	assert.Contains(t, string(contents), "sampctl build")
	assert.NotContains(t, string(contents), "github.com/")
	assert.NotContains(t, string(contents), "your-github-user")
}

func TestFetchInitTemplatesReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	answers := Answers{Repo: "my-repo", PublishMode: publishGitHub}
	profile := starterProfile{GitIgnore: true, Readme: true, Editor: "vscode", EditorConfig: true}

	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	defer func() { http.DefaultTransport = oldTransport }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := fetchInitTemplates(ctx, tmpDir, profile, answers)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "failed to fetch template files")
}

func TestGeneratedHarnessContents(t *testing.T) {
	t.Parallel()

	output := string(generatedHarnessContents("openmp", []string{"include/foo.inc", "bar.inc"}))
	assert.Contains(t, output, "#include <open.mp>")
	assert.Contains(t, output, "#include \"include/foo.inc\"")
	assert.Contains(t, output, "#include \"bar.inc\"")
	assert.Contains(t, output, "main()")
}

func TestAppendUniqueDependencies(t *testing.T) {
	t.Parallel()

	deps := appendUniqueDependencies(
		[]versioning.DependencyString{"pawn-lang/samp-stdlib"},
		"pawn-lang/samp-stdlib",
		"Southclaws/zcmd",
	)
	assert.Equal(t,
		[]versioning.DependencyString{"pawn-lang/samp-stdlib", "Southclaws/zcmd"},
		deps,
	)
}
