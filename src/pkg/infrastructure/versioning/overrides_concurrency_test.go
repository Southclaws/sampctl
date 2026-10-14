package versioning

import (
	"sync"
	"testing"
)

func TestDependencyOverridesConcurrentLoadAndReset(t *testing.T) {
	ResetDependencyOverrides()

	var workers sync.WaitGroup
	for range 12 {
		workers.Go(func() {
			for range 20 {
				got := ApplyDependencyOverrides("Zeex/samp-plugin-crashdetect")
				if got != "AmyrAhmady/samp-plugin-crashdetect" {
					t.Errorf("ApplyDependencyOverrides() = %q, want built-in replacement", got)
				}

				ResetDependencyOverrides()
			}
		})
	}

	workers.Wait()
}
