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
