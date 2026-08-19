package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeprecatedPackageNotice(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "deprecated package",
			data: `{"deprecated":{}}`,
			want: "package is deprecated",
		},
		{
			name: "message and replacement",
			data: `{"deprecated":{"message":"Use the maintained package.","replacement":"new-user/new-package"}}`,
			want: "package is deprecated: Use the maintained package.; use new-user/new-package instead",
		},
		{
			name: "active package",
			data: `{"entry":"main.pwn"}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependencyPath := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dependencyPath, "pawn.json"), []byte(tt.data), 0o644))
			assert.Equal(t, tt.want, deprecatedPackageNotice(dependencyPath))
		})
	}
}
