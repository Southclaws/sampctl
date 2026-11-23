package compiler

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
)

func TestCompileSource(t *testing.T) {
	ensureCompilerSourceFixtures(t)
	compilerDir := newFakeCompilerDir(t)
	defaultArgs := []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"}
	syntaxProblems := build.Problems{
		{File: "script.pwn", Line: 1, Severity: build.ProblemError, Code: "001", Description: "invalid function or declaration"},
		{File: "script.pwn", Line: 3, Severity: build.ProblemError, Code: "001", Description: "invalid function or declaration"},
		{File: "script.pwn", Line: 2, Severity: build.ProblemWarning, Code: "203", Description: `symbol is never used: "a"`},
		{File: "script.pwn", Line: 2, Severity: build.ProblemError, Code: "013", Description: "no entry point (no public functions)"},
	}

	for _, tc := range []struct {
		name, fixture string
		args          []string
		relative      bool
		wantProblems  build.Problems
		wantResult    build.Result
		wantOutput    bool
	}{
		{name: "simple-pass", fixture: "build-simple-pass", wantOutput: true},
		{
			name: "simple-pass-d3", fixture: "build-simple-pass", args: []string{"-d3"},
			wantResult: build.Result{Header: 60, Code: 184, Data: 0, StackHeap: 16384, Estimate: 20, Total: 16628},
			wantOutput: true,
		},
		{name: "simple-fail", fixture: "build-simple-fail", wantProblems: syntaxProblems},
		{name: "simple-fail-rel", fixture: "build-simple-fail", relative: true, wantProblems: syntaxProblems},
		{
			name: "local-include-pass", fixture: "build-local-include-pass", args: defaultArgs,
			wantResult: build.Result{Header: 60, Code: 220, Data: 0, StackHeap: 16384, Estimate: 32, Total: 16664},
			wantOutput: true,
		},
		{
			name: "local-include-warn", fixture: "build-local-include-warn", args: defaultArgs,
			wantProblems: build.Problems{
				{File: "library.inc", Line: 6, Severity: build.ProblemWarning, Code: "203", Description: `symbol is never used: "b"`},
				{File: "script.pwn", Line: 5, Severity: build.ProblemWarning, Code: "203", Description: `symbol is never used: "a"`},
			},
			wantResult: build.Result{Header: 60, Code: 276, Data: 0, StackHeap: 16384, Estimate: 32, Total: 16720},
			wantOutput: true,
		},
		{
			name: "fatal", fixture: "build-fatal", args: defaultArgs,
			wantProblems: build.Problems{
				{File: "script.pwn", Line: 1, Severity: build.ProblemFatal, Code: "100", Description: `cannot read from file: "idonotexist"`},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := filepath.Join("tests", tc.fixture)
			output := filepath.Join(t.TempDir(), "script.amx")
			config := build.Config{
				WorkingDir: workingDir,
				Input:      filepath.Join(workingDir, "script.pwn"),
				Output:     output,
				Args:       tc.args,
				Version:    "3.10.8",
				Compiler:   build.CompilerConfig{Path: compilerDir},
			}

			problems, result, err := CompileSource(t.Context(), CompileRequest{
				ExecDir:  ".",
				CacheDir: t.TempDir(),
				Platform: runtime.GOOS,
				Config:   config,
				Relative: tc.relative,
			})

			require.NoError(t, err)
			wantProblems := append(build.Problems(nil), tc.wantProblems...)
			if !tc.relative {
				for i, problem := range wantProblems {
					wantProblems[i].File = fs.MustAbs(filepath.Join(workingDir, problem.File))
				}
			}
			assert.Equal(t, wantProblems, problems)
			assert.Equal(t, tc.wantResult, result)
			if tc.wantOutput {
				assert.FileExists(t, output)
			} else {
				assert.NoFileExists(t, output)
			}
		})
	}
}
