package pawnpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestOverridesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		format, absent, empty, populated string
	}{
		{"json", `{}`, `{"overrides":{}}`, `{"overrides":{"fixture/dep:v1":"fixture/dep:v2"}}`},
		{"yaml", "{}\n", "overrides: {}\n", "overrides:\n  fixture/dep:v1: fixture/dep:v2\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			pkg, err := PackageFromDefinition([]byte(tc.absent), tc.format)
			require.NoError(t, err)
			assert.Nil(t, pkg.Overrides)

			pkg, err = PackageFromDefinition([]byte(tc.empty), tc.format)
			require.NoError(t, err)
			assert.NotNil(t, pkg.Overrides)
			data, err := pkg.EncodeDefinition()
			require.NoError(t, err)
			assert.NotContains(t, string(data), "overrides")

			pkg, err = PackageFromDefinition([]byte(tc.populated), tc.format)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"fixture/dep:v1": "fixture/dep:v2"}, pkg.Overrides)
			pkg.LocalPath = t.TempDir()
			require.NoError(t, pkg.WriteDefinition())
			written, err := PackageFromDir(pkg.LocalPath)
			require.NoError(t, err)
			assert.Equal(t, pkg.Overrides, written.Overrides)
		})
	}
}

func TestManifestOverridesRejectDuplicateRules(t *testing.T) {
	for _, tc := range []struct {
		format, definition string
	}{
		{"json", `{"overrides":{"fixture/dep":"fixture/first","fixture/dep":"fixture/second"}}`},
		{"yaml", "overrides:\n  fixture/dep: fixture/first\n  fixture/dep: fixture/second\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			_, err := PackageFromDefinition([]byte(tc.definition), tc.format)
			require.ErrorContains(t, err, "fixture/dep")
		})
	}
}

func TestJSONManifestRejectsRepeatedOverridesFields(t *testing.T) {
	for _, fields := range []string{
		`"overrides":{"fixture/old":"fixture/one"},"overrides":{"fixture/old":"fixture/two"}`,
		`"overrides":null,"overrides":{}`,
		`"overrides":{},"overrides":null`,
	} {
		_, err := PackageFromDefinition([]byte("{"+fields+"}"), "json")
		require.ErrorContains(t, err, "multiple overrides fields")
	}
}

func TestDependencyManifestIgnoresOverrideMetadata(t *testing.T) {
	for _, tc := range []struct {
		format, definition string
	}{
		{"json", `{"repo":"child","overrides":{"fixture/old":42}}`},
		{"yaml", "repo: child\noverrides: [invalid]\n"},
		{"yaml", "defaults: &defaults\n  repo: child\n  overrides: [invalid]\n<<: *defaults\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			pkg, err := DependencyFromDefinition([]byte(tc.definition), tc.format)
			require.NoError(t, err)
			assert.Equal(t, "child", pkg.Repo)
			assert.Empty(t, pkg.Overrides)
			_, err = PackageFromDefinition([]byte(tc.definition), tc.format)
			require.Error(t, err)
		})
	}

	_, err := DependencyFromDefinition([]byte("&self {<<: *self}"), "yaml")
	require.Error(t, err)
}
