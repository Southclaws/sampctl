package print

import "testing"

func TestPrints(t *testing.T) {
	Verb("should not appear")
	SetVerbose()
	Verb("A Verbose message")
	Info("An info message")
	Warn("A warning message")
	Erro("An error message")
}
