package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	res "github.com/Southclaws/sampctl/src/pkg/package/resource"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsurePluginsReportsUnsupportedRuntime(t *testing.T) {
	t.Parallel()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	workDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "YSF", Tag: "v2.2"}
	resource := res.Resource{
		Name:     `^YSF\.so$`,
		Platform: "linux",
		Version:  "0.3.7",
		Plugins:  []string{"YSF.so"},
	}
	seedCachedSinglePluginAsset(t, cacheDir, meta, resource, "YSF.so")
	cfg := &run.Runtime{
		WorkingDir:  workDir,
		Platform:    "linux",
		Version:     "openmp",
		RuntimeType: run.RuntimeTypeOpenMP,
		PluginDeps:  []versioning.DependencyMeta{meta},
	}
	err := EnsurePlugins(EnsurePluginsRequest{Context: context.Background(), Config: cfg, CacheDir: cacheDir})
	var missing *PluginResourceNotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("EnsurePlugins(SA-MP resource, openmp) error = %v, want *PluginResourceNotFoundError", err)
	}
	if missing.Meta != meta || missing.Platform != "linux" || missing.Version != "openmp" {
		t.Errorf("EnsurePlugins(SA-MP resource, openmp) failure = %+v, want %s, linux, openmp", missing, meta)
	}
	if _, err := os.Stat(filepath.Join(workDir, "plugins", "YSF.so")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("EnsurePlugins(SA-MP resource, openmp) plugin stat error = %v, want no incompatible binary installed", err)
	}
	if len(cfg.Plugins) != 0 {
		t.Errorf("EnsurePlugins(SA-MP resource, openmp) plugins = %v, want none", cfg.Plugins)
	}

	_, selected, err := EnsureVersionedPluginCached(EnsureVersionedPluginCachedRequest{
		Context: context.Background(), Meta: meta, Platform: "linux", Version: "0.3.7", CacheDir: cacheDir,
	})
	if err != nil || selected == nil || selected.Version != "0.3.7" {
		t.Errorf("EnsureVersionedPluginCached(SA-MP resource, 0.3.7) = %v, %v, want matching cached resource", selected, err)
	}
}

func TestGetResourceReportsDefaultRuntime(t *testing.T) {
	t.Parallel()
	_, err := GetResource(nil, "linux", "")
	var missing *PluginResourceNotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("GetResource(no resources, linux, default) error = %v, want *PluginResourceNotFoundError", err)
	}
	if missing.Platform != "linux" || missing.Version != "0.3.7" {
		t.Errorf("GetResource(no resources, linux, default) failure = %+v, want linux, 0.3.7", missing)
	}
}
