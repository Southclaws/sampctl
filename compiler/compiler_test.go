package compiler

import (
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
				Input:      "./tests/valid.pwn",
				Output:     "./tests/valid.amx",
				Includes:   []string{},
				Version:    "3.10.4",
			}}, false},
		{"invalid", args{
			util.FullPath("./tests/cache"),
			Config{
				WorkingDir: ".",
				Input:      "./tests/invalid.pwn",
				Output:     "./tests/invalid.amx",
				Includes:   []string{},
				Version:    "3.10.4",
			}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompileSource(".", tt.args.cacheDir, tt.args.config)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, util.Exists(tt.args.config.Output))
			}
		})
	}
}
