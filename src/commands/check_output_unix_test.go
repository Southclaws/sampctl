//go:build linux || darwin

package commands

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestCheckOutputStyleUsesActualTerminal(t *testing.T) {
	_, terminal := openCheckOutputTerminal(t)
	file, err := os.CreateTemp(t.TempDir(), "diagnostics")
	if err != nil {
		t.Fatalf("os.CreateTemp() error = %v", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close diagnostic file: %v", err)
		}
	})

	for _, tc := range []struct {
		name     string
		writer   io.Writer
		term     string
		noColor  string
		bare     bool
		coloured bool
	}{
		{name: "terminal", writer: terminal, term: "xterm-256color", coloured: true},
		{name: "NO_COLOR", writer: terminal, term: "xterm-256color", noColor: "1"},
		{name: "NO_COLOR zero", writer: terminal, term: "xterm-256color", noColor: "0"},
		{name: "dumb terminal", writer: terminal, term: "dumb"},
		{name: "bare terminal", writer: terminal, term: "xterm-256color", bare: true},
		{name: "buffer", writer: &bytes.Buffer{}, term: "xterm-256color"},
		{name: "redirected file", writer: file, term: "xterm-256color"},
		{name: "discarded output", writer: io.Discard, term: "xterm-256color"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TERM", tc.term)
			t.Setenv("NO_COLOR", tc.noColor)
			got := newCheckOutputStyle(tc.writer, tc.bare)
			if got.coloured != tc.coloured {
				t.Errorf("newCheckOutputStyle(%T, bare=%t).coloured = %t, want %t (TERM=%q, NO_COLOR=%q)", tc.writer, tc.bare, got.coloured, tc.coloured, tc.term, tc.noColor)
			}
		})
	}
}

func TestWriteCheckResultsTerminalColours(t *testing.T) {
	results := []diagnostic{
		{Status: checkPassed, Message: "Configuration is valid"},
		{Status: checkFailed, Message: "Compiler is missing", Action: "Run sampctl build."},
		{Status: checkWarning, Message: "Git is unavailable"},
		{Status: checkSkipped, Message: "No lockfile found"},
		{Status: checkRepaired, Message: "Cache metadata removed"},
	}
	ansi := regexp.MustCompile("\x1b\\[[0-9;]*m")
	for _, tc := range []struct {
		name     string
		term     string
		noColor  string
		bare     bool
		coloured bool
	}{
		{name: "terminal", term: "xterm-256color", coloured: true},
		{name: "NO_COLOR", term: "xterm-256color", noColor: "1"},
		{name: "dumb terminal", term: "dumb"},
		{name: "bare terminal", term: "xterm-256color", bare: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TERM", tc.term)
			t.Setenv("NO_COLOR", tc.noColor)
			master, terminal := openCheckOutputTerminal(t)
			code, err := writeCheckResults(terminal, results, checkOutputOptions{Bare: tc.bare})
			if err != nil {
				t.Fatalf("writeCheckResults(terminal) error = %v", err)
			}
			if code != 1 {
				t.Errorf("writeCheckResults(terminal) exit code = %d, want 1", code)
			}
			const marker = "\x00sampctl-check-output-end\x00"
			if _, err := terminal.WriteString(marker); err != nil {
				t.Fatalf("write terminal marker: %v", err)
			}
			output := readCheckOutputTerminal(t, master, marker)
			if got := strings.Contains(output, "\x1b["); got != tc.coloured {
				t.Errorf("writeCheckResults(terminal) contains ANSI colours = %t, want %t; output = %q", got, tc.coloured, output)
			}
			plain := ansi.ReplaceAllString(output, "")
			for _, result := range results {
				if !strings.Contains(plain, result.Message) {
					t.Errorf("writeCheckResults(terminal) output = %q, want message %q", plain, result.Message)
				}
			}
			if !tc.bare {
				for _, row := range []string{
					"  ✓ Configuration is valid",
					"  ✗ Compiler is missing",
					"  ! Warning: Git is unavailable",
					"  – Skipped: No lockfile found",
					"  ✓ Repaired: Cache metadata removed",
				} {
					if !strings.Contains(plain, row) {
						t.Errorf("writeCheckResults(terminal) output = %q, want row %q", plain, row)
					}
				}
			}
		})
	}
}

func TestCheckCommandTerminalFlags(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	for _, tc := range []struct {
		name     string
		args     []string
		coloured bool
		verbose  bool
	}{
		{name: "normal", args: []string{"check"}, coloured: true},
		{name: "global bare", args: []string{"--bare", "check"}},
		{name: "local bare", args: []string{"check", "--bare"}},
		{name: "global verbose", args: []string{"--verbose", "check"}, coloured: true, verbose: true},
		{name: "local verbose", args: []string{"check", "--verbose"}, coloured: true, verbose: true},
		{name: "global bare and verbose", args: []string{"--bare", "--verbose", "check"}, verbose: true},
		{name: "local bare and verbose", args: []string{"check", "--bare", "--verbose"}, verbose: true},
		{name: "global bare local verbose", args: []string{"--bare", "check", "--verbose"}, verbose: true},
		{name: "global verbose local bare", args: []string{"--verbose", "check", "--bare"}, verbose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			projectPath := filepath.Join(dir, "pawn.json")
			if err := os.WriteFile(projectPath, []byte(`{}`), 0o600); err != nil {
				t.Fatalf("write project configuration: %v", err)
			}
			cacheDir := filepath.Join(t.TempDir(), "missing")
			app := newCLIApp("1.2.3", newCommandState("1.2.3", cacheDir))
			master, terminal := openCheckOutputTerminal(t)
			app.Writer, app.ErrWriter = terminal, terminal
			args := append([]string{"sampctl"}, tc.args...)
			args = append(args, "--dir", dir)
			err := app.Run(args)
			if code := ExitCode(err); code != 1 {
				t.Fatalf("app.Run(%q) exit code = %d, want 1 for the missing managed compiler; error = %v", args, code, err)
			}
			const marker = "\x00sampctl-check-command-end\x00"
			if _, err := terminal.WriteString(marker); err != nil {
				t.Fatalf("write terminal marker: %v", err)
			}
			output := readCheckOutputTerminal(t, master, marker)
			if got := strings.Contains(output, "\x1b["); got != tc.coloured {
				t.Errorf("app.Run(%q) terminal output contains ANSI colours = %t, want %t; output = %q", args, got, tc.coloured, output)
			}
			if got := strings.Contains(output, "    Path: "+projectPath); got != tc.verbose {
				t.Errorf("app.Run(%q) terminal output contains verbose project path = %t, want %t; output = %q", args, got, tc.verbose, output)
			}
			if !strings.Contains(output, "Project configuration is valid") {
				t.Errorf("app.Run(%q) terminal output = %q, want project diagnostic", args, output)
			}
			if strings.Contains(output, "Verbose logging active") {
				t.Errorf("app.Run(%q) terminal output = %q, want only diagnostic output", args, output)
			}
			if _, err := os.Stat(cacheDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("app.Run(%q) created configuration directory %q; stat error = %v, want os.ErrNotExist", args, cacheDir, err)
			}
		})
	}
}

func openCheckOutputTerminal(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := terminal.Close(); err != nil {
			t.Errorf("close check terminal: %v", err)
		}
		if err := master.Close(); err != nil {
			t.Errorf("close check terminal master: %v", err)
		}
	})
	return master, terminal
}

func readCheckOutputTerminal(t *testing.T, master *os.File, marker string) string {
	t.Helper()
	fd := int(master.Fd())
	if err := unix.SetNonblock(fd, true); err != nil {
		t.Fatalf("set terminal master nonblocking: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var output bytes.Buffer
	buf := make([]byte, 4096)
	for !strings.HasSuffix(output.String(), marker) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("read terminal output timed out; output = %q, want final marker %q", output.String(), marker)
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(poll, int(remaining.Milliseconds())+1); err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			t.Fatalf("poll terminal output: %v", err)
		}
		if poll[0].Revents&unix.POLLIN == 0 {
			continue
		}
		n, err := unix.Read(fd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
				continue
			}
			t.Fatalf("read terminal output: %v", err)
		}
		output.Write(buf[:n])
	}
	return strings.TrimSuffix(output.String(), marker)
}
