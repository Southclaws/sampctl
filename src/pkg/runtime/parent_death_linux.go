package runtime

import "syscall"

func setRuntimeParentDeathSignal(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGTERM
}
