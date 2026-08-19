package pawnpackage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestPackageFromDefinitionLoadsDeprecationMetadata(t *testing.T) {
	tests := []struct {
		name   string
		format string
		data   string
	}{
		{
			name:   "json",
			format: "json",
			data:   `{"deprecated":{"message":"Use the maintained package.","replacement":"new-user/new-package"}}`,
		},
		{
			name:   "yaml",
			format: "yaml",
			data:   "deprecated:\n  message: Use the maintained package.\n  replacement: new-user/new-package\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := pawnpackage.PackageFromDefinition([]byte(tt.data), tt.format)
			require.NoError(t, err)
			require.NotNil(t, pkg.Deprecated)
			assert.Equal(t, "Use the maintained package.", pkg.Deprecated.Message)
			assert.Equal(t, "new-user/new-package", pkg.Deprecated.Replacement)
		})
	}
}

func TestDeprecationInfoNotice(t *testing.T) {
	tests := []struct {
		name string
		info pawnpackage.DeprecationInfo
		want string
	}{
		{
			name: "empty",
			want: "package is deprecated",
		},
		{
			name: "message",
			info: pawnpackage.DeprecationInfo{Message: "Use the maintained package."},
			want: "package is deprecated: Use the maintained package.",
		},
		{
			name: "replacement",
			info: pawnpackage.DeprecationInfo{Replacement: "new-user/new-package"},
			want: "package is deprecated: use new-user/new-package instead",
		},
		{
			name: "message and replacement",
			info: pawnpackage.DeprecationInfo{
				Message:     "Use the maintained package.",
				Replacement: "new-user/new-package",
			},
			want: "package is deprecated: Use the maintained package.; use new-user/new-package instead",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.info.Notice())
		})
	}
}
