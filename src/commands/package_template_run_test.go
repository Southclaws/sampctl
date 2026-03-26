package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestTemplateRunRuntimePreservesExistingConfig(t *testing.T) {
	t.Parallel()

	output := false
	base := &run.Runtime{
		Version:    "0.3.7",
		Plugins:    []run.Plugin{"mysql"},
		Components: []run.Plugin{"Pawn"},
		Output:     &output,
		Game:       map[string]any{"weather": "sunny"},
	}

	got := templateRunRuntime(base, "openmp", run.MainOnly)
	require.NotNil(t, got)
	require.NotSame(t, base, got)

	assert.Equal(t, "openmp", got.Version)
	assert.Equal(t, run.MainOnly, got.Mode)
	assert.Equal(t, base.Plugins, got.Plugins)
	assert.Equal(t, base.Components, got.Components)
	assert.Equal(t, base.Game, got.Game)
	require.NotNil(t, got.Output)
	assert.Equal(t, output, *got.Output)

	assert.Equal(t, "0.3.7", base.Version)
	assert.Equal(t, run.RunMode(""), base.Mode)
}

func TestTemplateRunRuntimeWithoutBase(t *testing.T) {
	t.Parallel()

	got := templateRunRuntime(nil, "openmp", run.MainOnly)
	require.NotNil(t, got)
	assert.Equal(t, "openmp", got.Version)
	assert.Equal(t, run.MainOnly, got.Mode)
}
