//go:build linux || darwin

package rook

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/config"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

type wizardAnswer struct {
	prompt string
	keys   string
}

func TestInitWorkflow(t *testing.T) {
	start := wizardAnswer{"How do you want to start this package?", "\r"}
	samp := wizardAnswer{"Which runtime should this package target?", "\r"}
	openmp := wizardAnswer{samp.prompt, "open.mp\r"}
	minimal := wizardAnswer{"Which starter setup do you want?", "Minimal\r"}
	withTemplates := wizardAnswer{minimal.prompt, "\r"}
	json := wizardAnswer{"Which package file format do you want?", "\r"}
	yaml := wizardAnswer{json.prompt, "YAML\r"}
	local := wizardAnswer{"Do you plan to release this as a package on GitHub?", "\r"}
	publish := wizardAnswer{local.prompt, "Yes\r"}
	repository := wizardAnswer{"GitHub repository to publish from (owner/repo)", "\x15new-owner/new-repo\r"}
	entry := wizardAnswer{"Which existing .pwn file should sampctl build?", "\r"}

	for _, tc := range []struct {
		name           string
		sourceSuffix   string
		blockManifest  bool
		wantTranscript string
		answers        []wizardAnswer
	}{
		{name: "interrupt", answers: []wizardAnswer{{start.prompt, "\x03"}}},
		{
			name: "entry interrupt", sourceSuffix: ".pwn",
			answers: []wizardAnswer{start, samp, minimal, json, publish, repository, {entry.prompt, "\x03"}},
		},
		{
			name: "existing source", sourceSuffix: ".pwn",
			answers:        []wizardAnswer{start, samp, minimal, json, local, entry},
			wantTranscript: "Found 2 pwn files and 0 inc files.",
		},
		{
			name: "include harness", sourceSuffix: ".inc",
			answers: []wizardAnswer{start, openmp, minimal, yaml, local},
		},
		{name: "minimal", answers: []wizardAnswer{start, samp, minimal, json, publish, repository}},
		{
			name: "manifest failure", blockManifest: true,
			answers: []wizardAnswer{start, samp, minimal, json, publish, repository},
		},
		{name: "template failure", answers: []wizardAnswer{start, samp, withTemplates, json, publish, repository}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.sourceSuffix != "" {
				for _, name := range []string{"z", "nested/a", "dependencies/ignored"} {
					file := filepath.Join(dir, name+tc.sourceSuffix)
					require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
					require.NoError(t, os.WriteFile(file, []byte("existing source"), 0o600))
				}
			}
			if tc.blockManifest {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "pawn.json"), 0o700))
			}

			transcript := runTestWizard(t, "TestInitWorkflowProcess", tc.answers, "SAMPCTL_INIT_SCENARIO="+tc.name, "SAMPCTL_INIT_DIR="+dir)
			if tc.wantTranscript != "" {
				require.Contains(t, transcript, tc.wantTranscript)
			}
		})
	}
}

func runTestWizard(t *testing.T, testName string, answers []wizardAnswer, env ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	binary, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, binary, "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), env...)
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = cmd.Stdout
	require.NoError(t, cmd.Start())
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()
	defer func() { _ = input.Close() }()

	var transcript strings.Builder
	next := 0
	cursorReports := 0
	buffer := make([]byte, 1024)
	for {
		n, readErr := output.Read(buffer)
		transcript.Write(buffer[:n])
		if next < len(answers) && strings.Contains(transcript.String(), answers[next].prompt) {
			_, err := io.WriteString(input, answers[next].keys)
			require.NoError(t, err)
			next++
		}
		// Keep answers before cursor reports so survey retains buffered input.
		for cursorReports < strings.Count(transcript.String(), "\x1b[6n") {
			_, err := io.WriteString(input, "\x1b[1;80R")
			require.NoError(t, err)
			cursorReports++
		}
		if readErr != nil {
			require.ErrorIs(t, readErr, io.EOF)
			break
		}
	}
	require.NoError(t, cmd.Wait(), transcript.String())
	require.Equal(t, len(answers), next, transcript.String())
	return transcript.String()
}

func TestInitWorkflowProcess(t *testing.T) {
	scenario := os.Getenv("SAMPCTL_INIT_SCENARIO")
	if scenario == "" {
		return
	}
	dir := os.Getenv("SAMPCTL_INIT_DIR")
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancel()
	cfg := &config.Config{DefaultUser: "original-owner"}
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if _, err := os.Stat(filepath.Join(dir, "pawn.json")); err != nil {
			return nil, err
		}
		return nil, context.Canceled
	})
	err := Init(InitOptions{Context: ctx, Dir: dir, Config: cfg, Preset: "samp"})
	if scenario == "interrupt" || scenario == "entry interrupt" {
		require.ErrorIs(t, err, terminal.InterruptErr)
		require.Equal(t, "original-owner", cfg.DefaultUser)
		require.NoFileExists(t, filepath.Join(dir, "pawn.json"))
		require.NoFileExists(t, filepath.Join(dir, "main.pwn"))
		return
	}
	if scenario == "manifest failure" {
		require.ErrorContains(t, err, "failed to write package definition")
		require.Equal(t, "new-owner", cfg.DefaultUser)
		require.FileExists(t, filepath.Join(dir, "main.pwn"))
		return
	}
	if scenario == "template failure" {
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorContains(t, err, "failed to fetch template files")
		require.DirExists(t, filepath.Join(dir, ".git"))
	} else {
		require.NoError(t, err)
		require.NoDirExists(t, filepath.Join(dir, ".git"))
	}

	pkg, err := pawnpackage.PackageFromDir(dir)
	require.NoError(t, err)
	if scenario == "include harness" {
		require.Equal(t, "openmp", pkg.Preset)
		require.Equal(t, "yaml", pkg.Format)
		require.Equal(t, "original-owner", cfg.DefaultUser)
		require.Equal(t, "original-owner", pkg.User)
		require.Equal(t, "test.pwn", pkg.Entry)
		require.Equal(t, "gamemodes/test.amx", pkg.Output)
		require.Equal(t, stdDependenciesForPreset("openmp"), pkg.Dependencies)
		content, err := os.ReadFile(filepath.Join(dir, pkg.Entry))
		require.NoError(t, err)
		require.Equal(t, generatedHarnessContents("openmp", []string{"nested/a.inc", "z.inc"}), content)
		return
	}
	require.Equal(t, "samp", pkg.Preset)
	require.Equal(t, "json", pkg.Format)
	require.Equal(t, "0.0.1", pkg.Tag)
	require.Equal(t, stdDependenciesForPreset("samp"), pkg.Dependencies)
	if scenario == "existing source" {
		require.Equal(t, "original-owner", cfg.DefaultUser)
		require.Equal(t, "original-owner", pkg.User)
		require.Equal(t, "nested/a.pwn", pkg.Entry)
		require.Equal(t, "gamemodes/a.amx", pkg.Output)
		content, err := os.ReadFile(filepath.Join(dir, pkg.Entry))
		require.NoError(t, err)
		require.Equal(t, "existing source", string(content))
	} else {
		require.Equal(t, "new-owner", cfg.DefaultUser)
		require.Equal(t, "new-owner", pkg.User)
		require.Equal(t, "new-repo", pkg.Repo)
		require.Equal(t, "main.pwn", pkg.Entry)
		require.Equal(t, "gamemodes/main.amx", pkg.Output)
		content, err := os.ReadFile(filepath.Join(dir, pkg.Entry))
		require.NoError(t, err)
		require.Equal(t, generatedGamemodeContents("samp"), content)
	}
	require.NoFileExists(t, filepath.Join(dir, "pawn.lock"), "dependency installation must be skipped without git")
}
