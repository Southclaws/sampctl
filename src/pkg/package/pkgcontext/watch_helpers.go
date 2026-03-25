package pkgcontext

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

func newTerminationSignals() (chan os.Signal, func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	return signals, func() {
		signal.Stop(signals)
	}
}

type watchDebouncer struct {
	pendingEvent string
	timer        *time.Timer
	ch           <-chan time.Time
}

func (d *watchDebouncer) Queue(eventName string, delay time.Duration) {
	d.pendingEvent = eventName
	if d.timer == nil {
		d.timer = time.NewTimer(delay)
		d.ch = d.timer.C
		return
	}

	if !d.timer.Stop() {
		select {
		case <-d.timer.C:
		default:
		}
	}
	d.timer.Reset(delay)
	d.ch = d.timer.C
}

func (d *watchDebouncer) Channel() <-chan time.Time {
	return d.ch
}

func (d *watchDebouncer) OnTimerFired(running bool) (string, bool) {
	d.ch = nil
	if d.pendingEvent == "" || running {
		return "", false
	}

	eventName := d.pendingEvent
	d.pendingEvent = ""
	return eventName, true
}

func (d *watchDebouncer) PopPending() (string, bool) {
	if d.pendingEvent == "" {
		return "", false
	}

	eventName := d.pendingEvent
	d.pendingEvent = ""
	return eventName, true
}

func (d *watchDebouncer) Stop() {
	if d.timer == nil {
		return
	}
	if !d.timer.Stop() {
		select {
		case <-d.timer.C:
		default:
		}
	}
	d.ch = nil
}

type watchedRuntime struct {
	running atomic.Bool
	cancel  context.CancelFunc
	done    <-chan error
}

func (w *watchedRuntime) Restart(parent context.Context, starter func(context.Context, *atomic.Bool) <-chan error) {
	w.Stop()
	runCtx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.done = starter(runCtx, &w.running)
}

func (w *watchedRuntime) Stop() {
	if w.done == nil || !w.running.Load() {
		return
	}

	fmt.Println("watch-run: killing existing runtime process")
	w.cancel()
	<-w.done
	fmt.Println("watch-run: killed existing runtime process")
	w.cancel = nil
	w.done = nil
}
