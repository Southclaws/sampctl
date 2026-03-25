package compiler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/build"
)

func TestBuildIncludeArgs(t *testing.T) {
	t.Run("deduplicates include paths", func(t *testing.T) {
		execDir := t.TempDir()
		incDir := filepath.Join(execDir, "include")
		require.NoError(t, os.MkdirAll(incDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(incDir, "a.inc"), []byte(""), 0o644))

		args, err := buildIncludeArgs(execDir, []string{"include", "include"})
		require.NoError(t, err)
		assert.Equal(t, []string{"-i" + incDir}, args)
	})

	t.Run("detects duplicate include files across paths", func(t *testing.T) {
		execDir := t.TempDir()
		incA := filepath.Join(execDir, "a")
		incB := filepath.Join(execDir, "b")
		require.NoError(t, os.MkdirAll(incA, 0o755))
		require.NoError(t, os.MkdirAll(incB, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(incA, "shared.inc"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(incB, "shared.inc"), []byte(""), 0o644))

		_, err := buildIncludeArgs(execDir, []string{"a", "b"})
		require.ErrorContains(t, err, "conflicting filenames")
	})

	t.Run("returns error for missing include dir", func(t *testing.T) {
		_, err := buildIncludeArgs(t.TempDir(), []string{"missing"})
		require.Error(t, err)
	})
}

func TestBuildConstantArgs(t *testing.T) {
	t.Setenv("BUILD_ENV", "env-value")

	args := buildConstantArgs(map[string]string{
		"NUMBER": "42",
		"FLOAT":  "3.14",
		"EMPTY":  "",
		"TEXT":   `hello "quoted"`,
		"ENV":    "$BUILD_ENV",
		"UNSET":  "$MISSING_ENV_FOR_TEST",
	})

	assert.Contains(t, args, `NUMBER=42`)
	assert.Contains(t, args, `FLOAT=3.14`)
	assert.Contains(t, args, `EMPTY=`)
	assert.Contains(t, args, `ENV="env-value"`)
	assert.Contains(t, args, `TEXT="hello \\"quoted\\""`)
	assert.Contains(t, args, `UNSET=`)
	assert.Equal(t, "plain", resolveConstantValue("plain"))
	assert.Equal(t, "env-value", resolveConstantValue("$BUILD_ENV"))
	assert.True(t, isNumeric("10"))
	assert.True(t, isNumeric("1.25"))
	assert.False(t, isNumeric("abc"))
}

func TestRunBuildCommands(t *testing.T) {
	t.Run("pre and post build commands write output", func(t *testing.T) {
		cfg := build.Config{
			PreBuildCommands:  [][]string{{"/bin/sh", "-c", "printf pre"}},
			PostBuildCommands: [][]string{{"/bin/sh", "-c", "printf post"}},
		}

		var output bytes.Buffer
		require.NoError(t, RunPreBuildCommands(context.Background(), cfg, &output))
		require.NoError(t, RunPostBuildCommands(context.Background(), cfg, &output))
		assert.Equal(t, "prepost", output.String())
	})

	t.Run("returns command error", func(t *testing.T) {
		cfg := build.Config{PreBuildCommands: [][]string{{"/bin/sh", "-c", "exit 3"}}}
		err := RunPreBuildCommands(context.Background(), cfg, &bytes.Buffer{})
		require.Error(t, err)
	})
}
