package pawnpackage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestValidateRejectsNullConfigurations(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		format   string
		contents string
		field    string
	}{
		{format: "json", contents: `{"builds":[null]}`, field: "builds"},
		{format: "json", contents: `{"runtimes":[null]}`, field: "runtimes"},
		{format: "yaml", contents: "builds: [null]\n", field: "builds"},
		{format: "yaml", contents: "runtimes: [null]\n", field: "runtimes"},
	} {
		t.Run(tc.format+"/"+tc.field, func(t *testing.T) {
			pkg, err := PackageFromDefinition([]byte(tc.contents), tc.format)
			require.NoError(t, err)
			require.ErrorContains(t, pkg.Validate(), tc.field+" contains a null configuration")
		})
	}
}

func TestConfigSelectionHandlesUnvalidatedNullEntries(t *testing.T) {
	t.Parallel()

	pkg := Package{
		Builds:   []*build.Config{nil, {Name: "release", Output: "release.amx"}},
		Runtimes: []*run.Runtime{nil, {Name: "release", Version: "openmp"}},
	}

	assert.NotPanics(t, func() {
		assert.NotNil(t, pkg.GetBuildConfig(""))
		_, err := pkg.GetRuntimeConfig("")
		require.NoError(t, err)
	})

	assert.Equal(t, "release.amx", pkg.GetBuildConfig("release").Output)
	cfg, err := pkg.GetRuntimeConfig("release")
	require.NoError(t, err)
	assert.Equal(t, "openmp", cfg.Version)
}
