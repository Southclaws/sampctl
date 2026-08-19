package run

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunModeOutputTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    RunMode
		want    time.Duration
		wantErr bool
	}{
		{name: "server mode", mode: Server},
		{name: "timeout mode", mode: "timeout:5s", want: 5 * time.Second},
		{name: "millisecond timeout", mode: "timeout:250ms", want: 250 * time.Millisecond},
		{name: "empty duration", mode: "timeout:", wantErr: true},
		{name: "zero duration", mode: "timeout:0s", wantErr: true},
		{name: "negative duration", mode: "timeout:-1s", wantErr: true},
		{name: "invalid duration", mode: "timeout:later", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.mode.OutputTimeout()
			if tt.wantErr {
				require.Error(t, err)
				assert.Zero(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeValidateRejectsInvalidOutputTimeout(t *testing.T) {
	t.Parallel()

	cfg := Runtime{
		WorkingDir: "/tmp/project",
		Platform:   "linux",
		Format:     "json",
		Version:    "0.3.7",
		Mode:       "timeout:0s",
	}

	assert.ErrorContains(t, cfg.Validate(), "duration must be positive")
}
