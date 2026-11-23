package print

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintsPlainMessages(t *testing.T) {
	resetPrintFlagsForTest(t)

	assert.Empty(t, captureStdout(t, func() { Verb("hidden") }))

	SetVerbose()
	assert.Contains(t, captureStdout(t, func() { Verb("shown") }), "INFO: shown")
	assert.Contains(t, captureStdout(t, func() { Info("info") }), "INFO: info")
	assert.Contains(t, captureStdout(t, func() { Warn("warn") }), "WARN: warn")
	assert.Contains(t, captureStdout(t, func() { Erro("error") }), "ERROR: error")
}
