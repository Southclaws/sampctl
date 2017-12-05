package compiler

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/util"
)

func TestCompileSource(t *testing.T) {
	type args struct {
		cacheDir string
		config   Config
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{
			util.FullPath("./tests/cache"),
			Config{
				WorkingDir: ".",
				Input:      "./tests/compile/compile_test.pwn",
				Output:     "./tests/compile/compile_test.amx",
				Includes:   []string{},
				Version:    "3.10.4",
			}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CompileSource(".", tt.args.cacheDir, tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("CompileSource() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.True(t, util.Exists("./tests/compile/compile_test.amx"))
		})
	}
}

func TestMergeDefault(t *testing.T) {
	type args struct {
		config Config
	}
	tests := []struct {
		name       string
		args       args
		wantResult Config
	}{
		{"valid", args{Config{Args: []string{"-l"}}}, Config{
			Args:    []string{"-d3", "-;+", "-(+", "-Z+", "-l"},
			Version: "3.10.4",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult := MergeDefault(tt.args.config)
			sort.Strings(tt.wantResult.Args)
			sort.Strings(gotResult.Args)
			assert.Equal(t, tt.wantResult, gotResult)
		})
	}
}
