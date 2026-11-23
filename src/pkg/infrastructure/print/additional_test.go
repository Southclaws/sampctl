package print

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintsColouredMessages(t *testing.T) {
	resetPrintFlagsForTest(t)
	SetColoured()

	assert.Contains(t, captureStdout(t, func() { Info("colour") }), "INFO:")
	assert.Contains(t, captureStdout(t, func() { Warn("colour") }), "WARN:")
	assert.Contains(t, captureStdout(t, func() { Erro("colour") }), "ERROR:")
}

func resetPrintFlagsForTest(t *testing.T) {
	t.Helper()

	verbose, coloured := isVerbose.Load(), isColoured.Load()
	t.Cleanup(func() {
		isVerbose.Store(verbose)
		isColoured.Store(coloured)
	})
	isVerbose.Store(false)
	isColoured.Store(false)
}

func captureStdout(t *testing.T, printMessage func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer func() { assert.NoError(t, reader.Close()) }()

	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
		_ = writer.Close()
	}()

	printMessage()
	require.NoError(t, writer.Close())
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(output)
}
