package runtime

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type notificationWriter struct {
	mu      sync.Mutex
	notices []chan struct{}
}

func (w *notificationWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.notices) > 0 {
		close(w.notices[0])
		w.notices = w.notices[1:]
	}
	return len(data), nil
}

func (w *notificationWriter) addNotice() <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	notice := make(chan struct{})
	w.notices = append(w.notices, notice)
	return notice
}

func TestWaitForRuntimeTerminationTimesOutWithoutOutput(t *testing.T) {
	t.Parallel()

	term := waitForRuntimeTermination(runtimeTerminationRequest{
		Context:       context.Background(),
		Output:        io.Discard,
		OutputTimeout: 20 * time.Millisecond,
		StreamCh:      make(chan string),
		TermCh:        make(chan termination),
	})

	require.Error(t, term.err)
	assert.EqualError(t, term.err, "runtime output timed out after 20ms")
	assert.True(t, term.exit)
}

func TestWaitForRuntimeTerminationResetsOutputTimeoutAfterOutput(t *testing.T) {
	t.Parallel()

	const outputTimeout = 200 * time.Millisecond
	streamCh := make(chan string)
	termCh := make(chan termination)
	writer := &notificationWriter{}
	termDone := make(chan termination, 1)
	go func() {
		termDone <- waitForRuntimeTermination(runtimeTerminationRequest{
			Context:       context.Background(),
			Output:        writer,
			OutputTimeout: outputTimeout,
			StreamCh:      streamCh,
			TermCh:        termCh,
		})
	}()

	firstWrite := writer.addNotice()
	streamCh <- "first"
	select {
	case <-firstWrite:
	case <-time.After(time.Second):
		t.Fatal("first output was not processed")
	}

	time.Sleep(outputTimeout / 2)
	secondWrite := writer.addNotice()
	streamCh <- "second"
	select {
	case <-secondWrite:
	case <-time.After(time.Second):
		t.Fatal("second output was not processed")
	}

	select {
	case term := <-termDone:
		t.Fatalf("timeout fired before the reset duration elapsed: %+v", term)
	case <-time.After(outputTimeout / 2):
	}

	select {
	case term := <-termDone:
		require.Error(t, term.err)
		assert.True(t, term.exit)
	case <-time.After(time.Second):
		t.Fatal("timeout did not fire after the final output")
	}
}
