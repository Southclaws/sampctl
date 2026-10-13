//go:build !linux && !windows

package runtime

import "syscall"

func setRuntimeParentDeathSignal(_ *syscall.SysProcAttr) {}
