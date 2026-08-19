package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestProcessOutputLineDetectsPluginLoadFailures(t *testing.T) {
	t.Parallel()

	lines := []struct {
		name string
		line string
	}{
		{
			name: "samp plugin failure",
			line: "Failed (plugins/mysql.so: cannot open shared object file: No such file or directory)",
		},
		{
			name: "openmp plugin failure",
			line: "[error] Failed to load plugin 'mysql.so': file not found",
		},
		{
			name: "openmp component failure",
			line: "Failed to load component: it is a SA-MP plugin, put it in plugins/ folder.",
		},
	}

	modes := []struct {
		name string
		mode run.RunMode
	}{
		{name: "server", mode: run.Server},
		{name: "main only", mode: run.MainOnly},
		{name: "testing", mode: run.YTesting},
	}

	for _, line := range lines {
		for _, mode := range modes {
			t.Run(line.name+"/"+mode.name, func(t *testing.T) {
				state := &outputModeState{}
				gotLine, emit, term, stop := processOutputLine(mode.mode, state, line.line)

				assert.Equal(t, line.line, gotLine)
				assert.True(t, emit)
				require.NotNil(t, term)
				assert.True(t, term.exit)
				assert.ErrorContains(t, term.err, "plugin load failed")
				assert.True(t, stop)
			})
		}
	}
}

func TestProcessOutputLineIgnoresUnrelatedFailure(t *testing.T) {
	t.Parallel()

	line, emit, term, stop := processOutputLine(run.Server, &outputModeState{}, "script failed to initialize")

	assert.Equal(t, "script failed to initialize", line)
	assert.True(t, emit)
	assert.Nil(t, term)
	assert.False(t, stop)
}
