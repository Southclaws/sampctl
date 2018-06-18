package analytics

import (
	"sync"
	"testing"
	"time"
)

func TestExecutorClose(t *testing.T) {
	// Simply make sure that nothing raises a panic nor blocks.
	ex := newExecutor(1)
	ex.close()
}

func TestExecutorSimple(t *testing.T) {
	wg := &sync.WaitGroup{}
	ex := newExecutor(1)
	defer ex.close()

	wg.Add(1)

	if !ex.do(wg.Done) {
		t.Error("failed pushing a task to an executor with a capacity of 1")
		return
	}

	// Make sure wg.Done gets called, this shouldn't block indefinitely.
	wg.Wait()
}

func TestExecutorMulti(t *testing.T) {
	wg := &sync.WaitGroup{}
	ex := newExecutor(3)
	defer ex.close()

	// Schedule a couple of tasks to fill the executor.
	for i := 0; i != 3; i++ {
		wg.Add(1)

		if !ex.do(func() {
			time.Sleep(10 * time.Millisecond)
			wg.Done()
		}) {
			t.Error("failed pushing a task to an executor with a capacity of 1")
			return
		}
	}

	// Make sure the executor refuses more tasks.
	if ex.do(func() {}) {
		t.Error("the executor should have been full and refused to run more tasks")
	}

	// Make sure wg.Done gets called, this shouldn't block indefinitely.
	wg.Wait()
}
