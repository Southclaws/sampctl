package print

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetColouredAndPrinting(t *testing.T) {
	origVerbose := isVerbose.Load()
	origColoured := isColoured.Load()
	defer func() {
		isVerbose.Store(origVerbose)
		isColoured.Store(origColoured)
	}()

	isVerbose.Store(false)
	isColoured.Store(false)

	assert.Equal(t, "", captureStdout(func() { Verb("hidden") }))

	SetVerbose()
	assert.Contains(t, captureStdout(func() { Verb("shown") }), "INFO: shown")
	assert.Contains(t, captureStdout(func() { Info("info") }), "INFO: info")
	assert.Contains(t, captureStdout(func() { Warn("warn") }), "WARN: warn")
	assert.Contains(t, captureStdout(func() { Erro("error") }), "ERROR: error")

	SetColoured()
	assert.Contains(t, captureStdout(func() { Info("colour") }), "INFO:")
	assert.Contains(t, captureStdout(func() { Warn("colour") }), "WARN:")
	assert.Contains(t, captureStdout(func() { Erro("colour") }), "ERROR:")
}

func captureStdout(fn func()) string {
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()
	_ = w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}
