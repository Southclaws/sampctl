package pawnpackage

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageFromDefinitionShapes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, format, contents string
		valid                  bool
		entry                  string
	}{
		{name: "JSON object", format: "json", contents: `{"entry":"main.pwn"}`, valid: true, entry: "main.pwn"},
		{name: "JSON empty object", format: "json", contents: `{}`, valid: true},
		{name: "JSON null", format: "json", contents: " \nnull\t"},
		{name: "JSON empty", format: "json"},
		{name: "JSON sequence", format: "json", contents: `[]`},
		{name: "JSON scalar", format: "json", contents: `"main.pwn"`},
		{name: "YAML mapping", format: "yaml", contents: "entry: main.pwn\n", valid: true, entry: "main.pwn"},
		{name: "YAML empty mapping", format: "yaml", contents: "{}\n", valid: true},
		{name: "YAML null", format: "yaml", contents: "null\n"},
		{name: "YAML implicit null", format: "yaml", contents: "~\n"},
		{name: "YAML empty", format: "yaml"},
		{name: "YAML comments", format: "yaml", contents: "# no configuration\n"},
		{name: "YAML sequence", format: "yaml", contents: "- main.pwn\n"},
		{name: "YAML scalar", format: "yaml", contents: "main.pwn\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := PackageFromDefinition([]byte(tc.contents), tc.format)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.format, pkg.Format)
			assert.Equal(t, tc.entry, pkg.Entry)
		})
	}
}

func TestPackageFromDefinitionPreservesJSONErrors(t *testing.T) {
	t.Parallel()

	_, err := PackageFromDefinition([]byte(`{"entry":`), "json")
	var syntaxErr *json.SyntaxError
	assert.True(t, errors.As(err, &syntaxErr))

	_, err = PackageFromDefinition([]byte(`{"entry":12}`), "json")
	var typeErr *json.UnmarshalTypeError
	assert.True(t, errors.As(err, &typeErr))
}
