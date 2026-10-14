package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestTemplateCommandsRejectCompilerDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("compiler fixture requires a posix shell")
	}

	configHome := t.TempDir()
	t.Setenv("HOME", configHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("APPDATA", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)

	cacheDir, err := fs.ConfigDir()
	require.NoError(t, err)

	for _, command := range []string{"build", "run"} {
		for _, severity := range []string{"error", "fatal error"} {
			t.Run(command+"/"+severity, func(t *testing.T) {
				templateDir := filepath.Join(cacheDir, "templates", "diagnostics")
				compilerDir := t.TempDir()
				require.NoError(t, os.MkdirAll(templateDir, 0o700))
				compiler := fmt.Sprintf("#!/bin/sh\nprintf 'tmpl.pwn(1) : %s 100: missing include\\n'\nexit 1\n", severity)
				require.NoError(t, os.WriteFile(filepath.Join(compilerDir, "pawncc"), []byte(compiler), 0o700))

				definition, err := json.Marshal(pawnpackage.Package{
					Entry: "tmpl.pwn", Output: "tmpl.amx",
					Build: &build.Config{Compiler: build.CompilerConfig{Path: compilerDir}},
				})
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(templateDir, "pawn.json"), definition, 0o600))
				source := filepath.Join(t.TempDir(), "script.pwn")
				require.NoError(t, os.WriteFile(source, []byte("main() {}\n"), 0o600))

				app := newCLIApp("test", newCommandState("test", cacheDir))
				app.Before, app.After = nil, nil
				err = app.Run([]string{"sampctl", "template", command, "diagnostics", source})
				if command == "run" {
					require.EqualError(t, err, "cannot run with build errors")
				} else {
					require.EqualError(t, err, "template build failed with compiler diagnostics")
				}
			})
		}
	}
}
