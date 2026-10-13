package runtime

import (
	"syscall"
	"testing"
)

func TestRuntimeParentDeathSignal(t *testing.T) {
	for _, tc := range []struct {
		name string
		attr func() *syscall.SysProcAttr
	}{
		{name: "fallback", attr: getRuntimeSysProcAttr},
		{name: "pty", attr: getRuntimePtySysProcAttr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if attr := tc.attr(); attr.Pdeathsig != syscall.SIGTERM {
				t.Errorf("runtime %s parent-death signal = %v, want SIGTERM", tc.name, attr.Pdeathsig)
			}
		})
	}
}
