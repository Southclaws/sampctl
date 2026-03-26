package print

import "testing"

func TestPrints(t *testing.T) {
	origVerbose := isVerbose.Load()
	origColoured := isColoured.Load()
	defer func() {
		isVerbose.Store(origVerbose)
		isColoured.Store(origColoured)
	}()

	isVerbose.Store(false)
	isColoured.Store(false)

	Verb("should not appear")
	SetVerbose()
	Verb("A Verbose message")
	Info("An info message")
	Warn("A warning message")
	Erro("An error message")
}
