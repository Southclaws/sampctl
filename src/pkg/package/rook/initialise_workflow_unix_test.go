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

func TestInitWorkflow(t *testing.T) {
	for _, scenario := range []string{"interrupt", "entry interrupt", "existing source", "include harness", "minimal", "manifest failure", "template failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"z.pwn", "nested/a.pwn", "dependencies/ignored.pwn"} {
				if scenario != "existing source" && scenario != "entry interrupt" && scenario != "include harness" {
					break
				}
				if scenario == "include harness" {
					name = strings.ReplaceAll(name, ".pwn", ".inc")
				}
				file := filepath.Join(dir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
				require.NoError(t, os.WriteFile(file, []byte("existing source"), 0o600))
			}
			if scenario == "manifest failure" {
				require.NoError(t, os.Mkdir(filepath.Join(dir, "pawn.json"), 0o700))
			}

			questions := []string{
				"How do you want to start this package?",
				"Which runtime should this package target?",
				"Which starter setup do you want?",
				"Which package file format do you want?",
				"Do you plan to release this as a package on GitHub?",
			}
			keys := []string{"\r", "\r", "Minimal\r", "\r", "\r"}
			if scenario == "interrupt" {
				questions, keys = questions[:1], []string{"\x03"}
			} else if scenario != "existing source" && scenario != "include harness" {
				keys[4] = "Yes\r"
				questions = append(questions, "GitHub repository to publish from (owner/repo)")
				keys = append(keys, "\x15new-owner/new-repo\r")
				if scenario == "entry interrupt" {
					questions = append(questions, "Which existing .pwn file should sampctl build?")
					keys = append(keys, "\x03")
				}
			}
			if scenario == "existing source" {
				questions = append(questions, "Which existing .pwn file should sampctl build?")
				keys = append(keys, "\r")
			}
			if scenario == "template failure" {
				keys[2] = "\r"
			}
			if scenario == "include harness" {
				keys[1], keys[3] = "open.mp\r", "YAML\r"
			}

			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			binary, err := os.Executable()
			require.NoError(t, err)
			cmd := exec.CommandContext(ctx, binary, "-test.run=^TestInitWorkflowProcess$")
			cmd.Env = append(os.Environ(), "SAMPCTL_INIT_SCENARIO="+scenario, "SAMPCTL_INIT_DIR="+dir)
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
				if next < len(questions) && strings.Contains(transcript.String(), questions[next]) {
					_, err := io.WriteString(input, keys[next])
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
			require.Equal(t, len(questions), next, transcript.String())
			if scenario == "existing source" {
				require.Contains(t, transcript.String(), "Found 2 pwn files and 0 inc files.")
			}
		})
	}
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
