package commands

import (
	"context"
	"os/signal"
	"syscall"
	"time"
)

func newCommandContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

func newCommandTimeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, stop := newCommandContext()
	if timeout <= 0 {
		return ctx, stop
	}

	timedCtx, cancel := context.WithTimeout(ctx, timeout)
	return timedCtx, func() {
		cancel()
		stop()
	}
}
