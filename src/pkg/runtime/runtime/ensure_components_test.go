package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
)

func TestEnsureComponentsExtractToComponentsDir(t *testing.T) {
	cfg := run.Runtime{
		Platform:    "linux",
		Version:     "v1.0.0-openmp",
		RuntimeType: run.RuntimeTypeOpenMP,
		PluginDeps: []versioning.DependencyMeta{{
			Scheme: "component",
			User:   "katursis",
			Repo:   "Pawn.RakNet",
			Tag:    "1.6.0-omp",
		}},
	}

	cfg.WorkingDir = filepath.Join("./tests/ensure", "Pawn.RakNet-component-linux-openmp")
	_ = os.MkdirAll(cfg.WorkingDir, 0o700)

	err := EnsurePlugins(context.Background(), gh, &cfg, "./tests/cache", true)
	assert.NoError(t, err)

	// It should install binaries into ./components
	componentsDir := filepath.Join(cfg.WorkingDir, "components")
	pluginsDir := filepath.Join(cfg.WorkingDir, "plugins")

	assert.True(t, fs.Exists(componentsDir))
	assert.True(t, fs.Exists(pluginsDir))

	entries, readErr := os.ReadDir(componentsDir)
	assert.NoError(t, readErr)

	foundBinary := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".so" || filepath.Ext(name) == ".dll" {
			foundBinary = true
			break
		}
	}
	assert.True(t, foundBinary, "expected at least one component binary in ./components")

	assert.NotEmpty(t, cfg.Components)
}
