package runtime

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestShouldKillTrackedProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		term termination
		want bool
	}{
		{
			name: "explicit exit",
			term: termination{exit: true},
			want: true,
		},
		{
			name: "signal error",
			term: termination{err: errors.New("received signal: interrupt")},
			want: true,
		},
		{
			name: "context canceled",
			term: termination{err: context.Canceled},
			want: true,
		},
		{
			name: "deadline exceeded",
			term: termination{err: context.DeadlineExceeded},
			want: true,
		},
		{
			name: "startup failure",
			term: termination{err: errors.New("failed to start server")},
			want: false,
		},
		{
			name: "clean termination",
			term: termination{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, shouldKillTrackedProcess(tt.term))
		})
	}
}
