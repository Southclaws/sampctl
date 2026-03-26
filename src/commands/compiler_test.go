package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilerListPrintsPresetsAndUsage(t *testing.T) {
	output := captureStdout(t, func() {
		err := compilerList(newGlobalCommandContext(t, nil))
		require.NoError(t, err)
	})

	assert.Contains(t, output, "PRESET")
	assert.Contains(t, output, "DESCRIPTION")
	assert.Contains(t, output, "REPOSITORY")
	assert.Contains(t, output, "openmp")
	assert.Contains(t, output, "samp")
	assert.Contains(t, output, "github.com/openmultiplayer/compiler")
	assert.Contains(t, output, "github.com/pawn-lang/compiler")
	assert.Contains(t, output, "Usage: Set the 'preset' field in your build configuration")
	assert.Contains(t, output, `"preset": "openmp"`)
	assert.Contains(t, output, `"version": "3.10.10"`)
}
