package pkgcontext

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchDebouncerFiresLatestEvent(t *testing.T) {
	t.Parallel()

	var debouncer watchDebouncer
	debouncer.Queue("first.pwn", time.Minute)
	debouncer.Queue("second.pwn", time.Minute)
	t.Cleanup(debouncer.Stop)

	eventName, ok := debouncer.OnTimerFired(false)
	require.True(t, ok)
	assert.Equal(t, "second.pwn", eventName)
	assert.Nil(t, debouncer.Channel())

	_, ok = debouncer.PopPending()
	assert.False(t, ok)
}

func TestWatchDebouncerRetainsPendingEventWhileRunning(t *testing.T) {
	t.Parallel()

	var debouncer watchDebouncer
	debouncer.Queue("main.pwn", time.Minute)
	t.Cleanup(debouncer.Stop)

	eventName, ok := debouncer.OnTimerFired(true)
	assert.False(t, ok)
	assert.Empty(t, eventName)
	assert.Nil(t, debouncer.Channel())

	eventName, ok = debouncer.PopPending()
	require.True(t, ok)
	assert.Equal(t, "main.pwn", eventName)
}

func TestWatchedRuntimeRestartStopsPreviousRun(t *testing.T) {
	t.Parallel()

	parent := context.Background()
	started := atomic.Int32{}
	stopped := atomic.Int32{}
	ready := make(chan struct{}, 2)

	starter := func(ctx context.Context, running *atomic.Bool) <-chan error {
		done := make(chan error, 1)
		started.Add(1)
		go func() {
			running.Store(true)
			ready <- struct{}{}
			<-ctx.Done()
			running.Store(false)
			stopped.Add(1)
			done <- ctx.Err()
			close(done)
		}()
		return done
	}

	var runtime watchedRuntime
	runtime.Restart(parent, starter)
	<-ready
	runtime.Restart(parent, starter)
	<-ready
	runtime.Stop()

	assert.Equal(t, int32(2), started.Load())
	assert.Equal(t, int32(2), stopped.Load())
	assert.Nil(t, runtime.done)
	assert.Nil(t, runtime.cancel)
}
