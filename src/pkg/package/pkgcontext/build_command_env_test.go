package pkgcontext

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPrintBuildCommandDoesNotExposeInheritedEnvironment(t *testing.T) {
	command := &exec.Cmd{
		Args: []string{"pawncc", "main.pwn", "-ogamemodes/main.amx", "-d3"},
		Env: []string{
			"GITHUB_TOKEN=synthetic-secret-token",
			"AWS_SECRET_ACCESS_KEY=synthetic-private-key",
			"HOME=/private/home-directory",
			"PATH=/private/search-path",
			"LD_LIBRARY_PATH_EXTRA=synthetic-unrelated-library-path",
			"LD_LIBRARY_PATH=/fixtures/compiler/linux",
			"DYLD_LIBRARY_PATH=/fixtures/compiler/darwin",
		},
	}
	output, err := os.CreateTemp(t.TempDir(), "build-command-output-")
	if err != nil {
		t.Fatal(err)
	}
	func() {
		original := os.Stdout
		os.Stdout = output
		defer func() {
			os.Stdout = original
			if err := output.Close(); err != nil {
				t.Errorf("close captured build command output: %v", err)
			}
		}()
		printBuildCommand(command)
	}()
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	printed := string(data)
	for _, expected := range append([]string{
		"LD_LIBRARY_PATH=/fixtures/compiler/linux",
		"DYLD_LIBRARY_PATH=/fixtures/compiler/darwin",
	}, command.Args...) {
		if !strings.Contains(printed, expected) {
			t.Errorf("printBuildCommand output is missing %q", expected)
		}
	}
	for _, secret := range []string{
		"synthetic-secret-token", "synthetic-private-key", "/private/home-directory",
		"/private/search-path", "synthetic-unrelated-library-path",
	} {
		if strings.Contains(printed, secret) {
			t.Errorf("printBuildCommand exposed unrelated environment value %q", secret)
		}
	}
}
