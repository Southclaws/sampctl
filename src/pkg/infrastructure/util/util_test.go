package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFileCreatesDestination(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	destination := filepath.Join(directory, "destination")
	require.NoError(t, os.WriteFile(source, nil, 0o600))

	err := CopyFile(source, destination)

	require.NoError(t, err)
	assert.True(t, Exists(destination))
}
