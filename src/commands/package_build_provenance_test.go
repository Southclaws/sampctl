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

	const workingCompiler = "#!/bin/sh\nfor arg do case \"$arg\" in -o*) printf 'compiled:%s' \"$1\" > \"${arg#-o}\";; esac; done\n"

	for _, test := range []struct {
		name              string
		input             string
		output            string
		compilerWorkDir   string
		compiler          string
		absolutePaths     bool
		externalCWD       bool
		selectFirstBuild  bool
		wantMissingOutput bool
		wantDirectory     bool
	}{
		{name: "default"},
		{name: "input override", input: "release.pwn"},
		{name: "output override", output: "build/release.amx", compilerWorkDir: "compiler"},
		{name: "both overrides", input: "release.pwn", output: "build/release.amx"},
		{name: "absolute overrides", absolutePaths: true},
		{name: "external default", externalCWD: true},
		{name: "external named", input: "release.pwn", output: "build/release.amx", externalCWD: true},
		{name: "implicit first configuration", input: "release.pwn", output: "build/release.amx", selectFirstBuild: true},
		{name: "missing artifact", output: "build/release.amx", compiler: "#!/bin/sh\nexit 0\n", wantMissingOutput: true},
		{name: "artifact read failure", output: "build/release.amx", compiler: "#!/bin/sh\nfor arg do case \"$arg\" in -o*) mkdir \"${arg#-o}\";; esac; done\n", wantDirectory: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTestConfigHome(t)

			root := t.TempDir()
			input, output := test.input, test.output
			if test.absolutePaths {
				input = filepath.Join(root, "release.pwn")
				output = filepath.Join(t.TempDir(), "release.amx")
			}
			selected := &build.Config{
				Name: "release", Input: input, Output: output, WorkingDir: test.compilerWorkDir,
				Compiler: build.CompilerConfig{Path: "compiler"},
			}
			pkg := pawnpackage.Package{User: "fixture", Repo: "root", Entry: "main.pwn", Output: "default.amx"}
			arguments := []string{}
			if input == "" && output == "" {
				pkg.Build = selected
			} else {
				pkg.Builds = []*build.Config{selected}
				arguments = []string{"release"}
			}
			if test.selectFirstBuild {
				arguments = nil
				pkg.Builds = append(pkg.Builds, &build.Config{Name: "default", Compiler: build.CompilerConfig{Version: "unused"}})
			}
			definition, err := json.Marshal(pkg)
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(root, "compiler"), 0o755))
			compiler := test.compiler
			if compiler == "" {
				compiler = workingCompiler
			}
			for name, content := range map[string]string{
				"pawn.json": string(definition), "main.pwn": "main() {}", "release.pwn": "main() {}",
				"default.amx":     "stale default artifact",
				"compiler/pawncc": compiler,
			} {
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o755))
			}

			workingDir := root
			if test.externalCWD {
				workingDir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(workingDir, "default.amx"), []byte("wrong directory artifact"), 0o644))
			}
			t.Chdir(workingDir)
			flags := flag.NewFlagSet("build", flag.ContinueOnError)
			flags.String("dir", root, "")
			require.NoError(t, flags.Parse(arguments))
			ctx := cli.NewContext(newTestCLIContext("test").App, flags, nil)

			err = packageBuild(ctx)

			require.NoError(t, err)

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
			if test.wantMissingOutput || test.wantDirectory {
				assert.Empty(t, lf.Build.OutputHash)
				if test.wantMissingOutput {
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
