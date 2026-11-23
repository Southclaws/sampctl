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

			err = pkg.Validate()

			require.ErrorContains(t, err, tc.field+" contains a null configuration")
		})
	}
}

func TestConfigSelectionHandlesUnvalidatedNullEntries(t *testing.T) {
	t.Parallel()

	pkg := Package{
		Builds:   []*build.Config{nil, {Name: "release", Output: "release.amx"}},
		Runtimes: []*run.Runtime{nil, {Name: "release", Version: "openmp"}},
	}

	defaultBuild := pkg.GetBuildConfig("")
	_, defaultErr := pkg.GetRuntimeConfig("")
	namedBuild := pkg.GetBuildConfig("release")
	namedRuntime, namedErr := pkg.GetRuntimeConfig("release")

	assert.NotNil(t, defaultBuild)
	require.NoError(t, defaultErr)
	require.NotNil(t, namedBuild)
	assert.Equal(t, "release.amx", namedBuild.Output)
	require.NoError(t, namedErr)
	assert.Equal(t, "openmp", namedRuntime.Version)
}
