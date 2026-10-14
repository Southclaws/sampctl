package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestPackageBuildRecordsExecutedConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture compiler is a POSIX shell script")
	}

	for _, test := range []struct {
		name     string
		input    string
		output   string
		external bool
	}{
		{name: "default"},
		{name: "input override", input: "release.pwn"},
		{name: "output override", output: "build/release.amx"},
		{name: "both overrides", input: "release.pwn", output: "build/release.amx"},
		{name: "absolute overrides"},
		{name: "external default", external: true},
		{name: "external named", input: "release.pwn", output: "build/release.amx", external: true},
		{name: "implicit first configuration", input: "release.pwn", output: "build/release.amx"},
		{name: "missing artifact", output: "build/release.amx"},
		{name: "artifact read failure", output: "build/release.amx"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(configdir.Refresh)
			configHome := t.TempDir()
			t.Setenv("HOME", configHome)
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("APPDATA", configHome)
			configdir.Refresh()

			root := t.TempDir()
			input, output := test.input, test.output
			if test.name == "absolute overrides" {
				input = filepath.Join(root, "release.pwn")
				output = filepath.Join(t.TempDir(), "release.amx")
			}
			selected := &build.Config{Name: "release", Input: input, Output: output, Compiler: build.CompilerConfig{Path: "compiler"}}
			if test.name == "output override" {
				selected.WorkingDir = "compiler"
			}
			pkg := pawnpackage.Package{User: "fixture", Repo: "root", Entry: "main.pwn", Output: "default.amx"}
			arguments := []string{}
			if input == "" && output == "" {
				pkg.Build = selected
			} else {
				pkg.Builds = []*build.Config{selected}
				arguments = []string{"release"}
			}
			if test.name == "implicit first configuration" {
				arguments = nil
				pkg.Builds = append(pkg.Builds, &build.Config{Name: "default", Compiler: build.CompilerConfig{Version: "unused"}})
			}
			definition, err := json.Marshal(pkg)
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(root, "compiler"), 0o755))
			compiler := "#!/bin/sh\nfor arg do case \"$arg\" in -o*) printf 'compiled:%s' \"$1\" > \"${arg#-o}\";; esac; done\n"
			switch test.name {
			case "missing artifact":
				compiler = "#!/bin/sh\nexit 0\n"
			case "artifact read failure":
				compiler = "#!/bin/sh\nfor arg do case \"$arg\" in -o*) mkdir \"${arg#-o}\";; esac; done\n"
			}
			for name, content := range map[string]string{
				"pawn.json": string(definition), "main.pwn": "main() {}", "release.pwn": "main() {}",
				"default.amx":     "stale default artifact",
				"compiler/pawncc": compiler,
			} {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o755))
			}

			workingDir := root
			if test.external {
				workingDir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(workingDir, "default.amx"), []byte("wrong directory artifact"), 0o644))
			}
			t.Chdir(workingDir)
			flags := flag.NewFlagSet("build", flag.ContinueOnError)
			flags.String("dir", root, "")
			require.NoError(t, flags.Parse(arguments))
			ctx := cli.NewContext(newTestCLIContext("test").App, flags, nil)
			require.NoError(t, packageBuild(ctx))

			if input == "" {
				input = pkg.Entry
			}
			if output == "" {
				output = pkg.Output
			}
			inputPath, outputPath := input, output
			if !filepath.IsAbs(inputPath) {
				inputPath = filepath.Join(root, inputPath)
			}
			if !filepath.IsAbs(outputPath) {
				outputPath = filepath.Join(root, outputPath)
			}
			lf, err := lockfile.Load(root)
			require.NoError(t, err)
			require.NotNil(t, lf.Build)
			assert.Equal(t, input, lf.Build.Entry)
			assert.Equal(t, output, lf.Build.Output)
			assert.Empty(t, lf.Build.CompilerVersion)
			if test.name == "missing artifact" || test.name == "artifact read failure" {
				assert.Empty(t, lf.Build.OutputHash)
				if test.name == "missing artifact" {
					assert.NoFileExists(t, outputPath)
				} else {
					assert.DirExists(t, outputPath)
				}
				return
			}

			artifact, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			assert.Equal(t, "compiled:"+inputPath, string(artifact))
			hash := sha256.Sum256(artifact)
			assert.Equal(t, "sha256:"+hex.EncodeToString(hash[:]), lf.Build.OutputHash)
		})
	}
}
