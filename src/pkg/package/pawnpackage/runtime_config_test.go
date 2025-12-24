package pawnpackage

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

func TestGetRuntimeConfigRuntimeOverride(t *testing.T) {
	pkg := Package{
		Runtimes: []*run.Runtime{
			{Name: "default", Version: "0.3.7"},
		},
		Runtime: &run.Runtime{Version: "openmp"},
	}

	cfg, err := pkg.GetRuntimeConfig("")
	require.NoError(t, err)
	require.Equal(t, "openmp", cfg.Version)
}

func TestGetRuntimeConfigPresetOpenMPDefault(t *testing.T) {
	pkg := Package{Preset: "openmp"}

	cfg, err := pkg.GetRuntimeConfig("")
	require.NoError(t, err)
	require.Equal(t, "openmp", cfg.Version)
}

func TestGetBuildConfigPresetOpenMPDefault(t *testing.T) {
	pkg := Package{Preset: "openmp"}

	cfg := pkg.GetBuildConfig("")
	require.Equal(t, "openmp", cfg.Compiler.Preset)
}

func TestGetBuildConfigPresetInferredFromRuntime(t *testing.T) {
	pkg := Package{Runtime: &run.Runtime{Version: "openmp"}}

	cfg := pkg.GetBuildConfig("")
	require.Equal(t, "openmp", cfg.Compiler.Preset)
}

func TestEffectiveLocal_Inferred(t *testing.T) {
	pkg := Package{Parent: true, Local: nil}
	require.True(t, pkg.EffectiveLocal())

	pkg = Package{Parent: false, Local: nil}
	require.False(t, pkg.EffectiveLocal())
}

func TestEffectiveLocal_ExplicitOverride(t *testing.T) {
	trueVal := true
	falseVal := false

	pkg := Package{Parent: false, Local: &trueVal}
	require.True(t, pkg.EffectiveLocal())

	pkg = Package{Parent: true, Local: &falseVal}
	require.False(t, pkg.EffectiveLocal())
}
