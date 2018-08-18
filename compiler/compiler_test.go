package compiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
)

func TestCompileSource(t *testing.T) {
	type args struct {
		cacheDir string
		config   types.BuildConfig
		relative bool
	}
	tests := []struct {
		name         string
		args         args
		wantProblems types.BuildProblems
		wantResult   types.BuildResult
		wantErr      bool
		wantOutput   bool
	}{
		{"simple-pass", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-simple-pass",
				Input:      "./tests/build-simple-pass/script.pwn",
				Output:     "./tests/build-simple-pass/script.amx",
				Includes:   []string{},
				Version:    "3.10.8",
			}, false},
			nil,
			types.BuildResult{},
			false, true},
		{"simple-pass-d3", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-simple-pass",
				Input:      "./tests/build-simple-pass/script.pwn",
				Output:     "./tests/build-simple-pass/script.amx",
				Args:       []string{"-d3"},
				Includes:   []string{},
				Version:    "3.10.8",
			}, false},
			nil,
			types.BuildResult{
				Header:    60,
				Code:      184,
				Data:      0,
				StackHeap: 16384,
				Estimate:  20,
				Total:     16628,
			},
			false, true},
		{"simple-fail", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-simple-fail",
				Input:      "./tests/build-simple-fail/script.pwn",
				Output:     "./tests/build-simple-fail/script.amx",
				Includes:   []string{},
				Version:    "3.10.8",
			}, false},
			types.BuildProblems{
				{File: "script.pwn", Line: 1, Severity: types.ProblemError, Description: `invalid function or declaration`},
				{File: "script.pwn", Line: 3, Severity: types.ProblemError, Description: `invalid function or declaration`},
				{File: "script.pwn", Line: 2, Severity: types.ProblemWarning, Description: `symbol is never used: "a"`},
				{File: "script.pwn", Line: 2, Severity: types.ProblemError, Description: `no entry point (no public functions)`},
			},
			types.BuildResult{},
			false, false},
		{"simple-fail-rel", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-simple-fail",
				Input:      "./tests/build-simple-fail/script.pwn",
				Output:     "./tests/build-simple-fail/script.amx",
				Includes:   []string{},
				Version:    "3.10.8",
			}, true},
			types.BuildProblems{
				{File: "script.pwn", Line: 1, Severity: types.ProblemError, Description: `invalid function or declaration`},
				{File: "script.pwn", Line: 3, Severity: types.ProblemError, Description: `invalid function or declaration`},
				{File: "script.pwn", Line: 2, Severity: types.ProblemWarning, Description: `symbol is never used: "a"`},
				{File: "script.pwn", Line: 2, Severity: types.ProblemError, Description: `no entry point (no public functions)`},
			},
			types.BuildResult{},
			false, false},
		{"local-include-pass", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-local-include-pass",
				Input:      "./tests/build-local-include-pass/script.pwn",
				Output:     "./tests/build-local-include-pass/script.amx",
				Args:       []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
				Includes:   []string{},
				Version:    "3.10.8",
			}, false},
			nil,
			types.BuildResult{
				Header:    60,
				Code:      220,
				Data:      0,
				StackHeap: 16384,
				Estimate:  32,
				Total:     16664,
			},
			false, true},
		{"local-include-warn", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-local-include-warn",
				Input:      "./tests/build-local-include-warn/script.pwn",
				Output:     "./tests/build-local-include-warn/script.amx",
				Args:       []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
				Includes:   []string{},
				Version:    "3.10.8",
			}, false},
			types.BuildProblems{
				{File: "library.inc", Line: 6, Severity: types.ProblemWarning, Description: `symbol is never used: "b"`},
				{File: "script.pwn", Line: 5, Severity: types.ProblemWarning, Description: `symbol is never used: "a"`},
			},
			types.BuildResult{
				Header:    60,
				Code:      276,
				Data:      0,
				StackHeap: 16384,
				Estimate:  32,
				Total:     16720,
			},
			false, true},
		{"fatal", args{
			util.FullPath("./tests/cache"),
			types.BuildConfig{
				WorkingDir: "./tests/build-fatal",
				Input:      "./tests/build-fatal/script.pwn",
				Output:     "./tests/build-fatal/script.amx",
				Args:       []string{"-d3", "-;+", "-(+", "-\\+", "-Z+"},
				Includes:   []string{},
				Version:    "3.10.8",
			}, false},
			types.BuildProblems{
				{File: "script.pwn", Line: 1, Severity: types.ProblemFatal, Description: `cannot read from file: "idonotexist"`},
			},
			types.BuildResult{},
			false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.MkdirAll(tt.args.cacheDir, 0700)
			assert.NoError(t, err)

			gotProblems, gotResult, err := CompileSource(context.Background(), gh, ".", "", tt.args.cacheDir, runtime.GOOS, tt.args.config, tt.args.relative)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			for i, p := range tt.wantProblems {
				if !tt.args.relative {
					tt.wantProblems[i].File = util.FullPath(filepath.Join(tt.args.config.WorkingDir, p.File))
				}
			}

			assert.Equal(t, tt.wantProblems, gotProblems)
			assert.Equal(t, tt.wantResult, gotResult)

			if tt.wantOutput {
				assert.True(t, util.Exists(tt.args.config.Output))
			}
		})
	}
}
