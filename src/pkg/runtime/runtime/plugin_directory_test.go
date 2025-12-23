package runtime

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

func TestGetPluginDirectory(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		runtimeType run.RuntimeType
		expectedDir string
	}{
		{
			name:        "SA-MP runtime auto-detect",
			version:     "0.3.7",
			runtimeType: run.RuntimeTypeAuto,
			expectedDir: "plugins",
		},
		{
			name:        "Open.MP runtime auto-detect",
			version:     "v1.0.0-openmp",
			runtimeType: run.RuntimeTypeAuto,
			expectedDir: "components",
		},
		{
			name:        "Explicit SA-MP runtime",
			version:     "custom-version",
			runtimeType: run.RuntimeTypeSAMP,
			expectedDir: "plugins",
		},
		{
			name:        "Explicit Open.MP runtime",
			version:     "custom-version",
			runtimeType: run.RuntimeTypeOpenMP,
			expectedDir: "components",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &run.Runtime{
				Version:     tt.version,
				RuntimeType: tt.runtimeType,
				WorkingDir:  "/test/dir",
			}

			result := getPluginDirectory(cfg)
			assert.Equal(t, tt.expectedDir, result)

			// Also test the full path
			expectedPath := filepath.Join("/test/dir", tt.expectedDir)
			assert.Equal(t, expectedPath, filepath.Join(cfg.WorkingDir, result))
		})
	}
}
