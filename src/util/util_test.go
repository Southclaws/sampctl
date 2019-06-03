package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	os.MkdirAll("./tests", 0755)

	f, _ := os.Create("./tests/file")
	f.Close() // nolint

	os.Exit(m.Run())
}

func TestCopyFile(t *testing.T) {
	type args struct {
		src string
		dst string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"valid", args{"./tests/file", "./tests/file2"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CopyFile(tt.args.src, tt.args.dst)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, Exists(tt.args.dst))
			}
		})
	}
}
