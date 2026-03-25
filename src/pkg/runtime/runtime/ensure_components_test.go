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
	res "github.com/Southclaws/sampctl/src/resource"
)

func TestEnsureComponentsExtractToComponentsDir(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
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

	seedCachedPluginPackage(t, cacheDir, cfg.PluginDeps[0], pluginFixturePackage(cfg.PluginDeps[0], []res.Resource{{
		Name:     `^pawnraknet-1\.6\.0-omp\.tar\.gz$`,
		Platform: "linux",
		Archive:  true,
		Plugins:  []string{"plugins/Pawn.RakNet.so"},
	}}), "pawnraknet-1.6.0-omp.tar.gz", map[string]string{"plugins/Pawn.RakNet.so": "fixture"})

	cfg.WorkingDir = filepath.Join(t.TempDir(), "Pawn.RakNet-component-linux-openmp")

	err := EnsurePlugins(EnsurePluginsRequest{
		Context:  context.Background(),
		GitHub:   nil,
		Config:   &cfg,
		CacheDir: cacheDir,
		NoCache:  false,
	})
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
