package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsureBinariesKeepsExternalOwnershipWithoutLockfile(t *testing.T) {
	t.Parallel()
	project, external, cache := t.TempDir(), t.TempDir(), t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(project, "pawn.json"), "{}")
	writeRuntimeTestFile(t, filepath.Join(external, "server.cfg"), "user configuration")
	cfg := run.Runtime{Version: "first", Platform: "linux", WorkingDir: external}
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "first binary", "obsolete": "old library"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	if manifest, err := loadInstalledRuntimeManifest(external); err != nil || manifest == nil || manifest.Version != "first" {
		t.Fatalf("initial installed manifest = %+v, error = %v", manifest, err)
	}
	cfg.Version = "second"
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "second binary", "runtime-library": "new library"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, filepath.Join(external, "samp03svr"), "second binary")
	checkRuntimeTestFile(t, filepath.Join(external, "server.cfg"), "user configuration")
	if _, err := os.Lstat(filepath.Join(external, "obsolete")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("runtime upgrade retained obsolete file: %v", err)
	}
	manifestPath := filepath.Join(external, installedRuntimeManifestFileName)
	manifestInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	binaryInfo, err := os.Stat(filepath.Join(external, "samp03svr"))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
			t.Fatal(err)
		}
	}
	manifestAfter, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	binaryAfter, err := os.Stat(filepath.Join(external, "samp03svr"))
	if err != nil {
		t.Fatal(err)
	}
	if !manifestInfo.ModTime().Equal(manifestAfter.ModTime()) || !binaryInfo.ModTime().Equal(binaryAfter.ModTime()) {
		t.Error("repeated ensure rewrote a verified runtime or its ownership manifest")
	}
	info, err := GetRuntimeManifestInfo(external)
	if err != nil || info == nil || info.Version != "second" {
		t.Errorf("external runtime info = %+v, error = %v", info, err)
	}
	for _, root := range []string{project, external} {
		if _, err := os.Lstat(filepath.Join(root, lockfile.Filename)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("runtime provisioning created a lockfile at %s: %v", root, err)
		}
	}
}

func TestEnsureBinariesRetainsOwnershipDuringInterruptedUpgrade(t *testing.T) {
	t.Parallel()
	external, cache := t.TempDir(), t.TempDir()
	cfg := run.Runtime{Version: "first", Platform: "linux", WorkingDir: external}
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "first binary", "z-library": "first library"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(external, installedRuntimeManifestFileName)
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Version = "second"
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "second binary", "z-library": "second library"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interrupted := &runtimeUpgradeCancellationContext{Context: ctx, cancel: cancel, binary: filepath.Join(external, "samp03svr")}
	if _, err := EnsureBinariesContext(interrupted, cache, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted upgrade = %v, want context.Canceled", err)
	}
	checkRuntimeTestFile(t, filepath.Join(external, "samp03svr"), "second binary")
	checkRuntimeTestFile(t, filepath.Join(external, "z-library"), "first library")
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("interrupted upgrade replaced the previous ownership manifest")
	}
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, filepath.Join(external, "z-library"), "second library")
	manifest, err := loadInstalledRuntimeManifest(external)
	if err != nil || manifest == nil || manifest.Version != "second" {
		t.Errorf("retried upgrade manifest = %+v, error = %v", manifest, err)
	}
}

func TestEnsureBinariesValidatesAllStagedFilesForSameVersion(t *testing.T) {
	t.Parallel()
	root, cache := t.TempDir(), t.TempDir()
	cfg := run.Runtime{Version: "first", Platform: "linux", WorkingDir: root}
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "binary"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "binary", "library": "required library"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, filepath.Join(root, "library"), "required library")
}

func TestEnsureBinariesMigratesLegacyInstalledOwnership(t *testing.T) {
	t.Parallel()
	root, cache := t.TempDir(), t.TempDir()
	cfg := run.Runtime{Version: "first", Platform: "linux", WorkingDir: root}
	writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "first binary")
	manifest, err := buildRuntimeManifest(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	saveRuntimeTestLockfile(t, root, manifest)
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "first binary"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, lockfile.Filename)); err != nil {
		t.Fatal(err)
	}
	cfg.Version = "second"
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "second binary"})
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "second binary")
}

func TestEnsureBinariesPreservesUnsafeOwnershipMetadata(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"malformed", "directory", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root, cache := t.TempDir(), t.TempDir()
			cfg := run.Runtime{Version: "first", Platform: "linux", WorkingDir: root}
			seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "new binary"})
			writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "user binary")
			metadata := filepath.Join(root, installedRuntimeManifestFileName)
			switch scenario {
			case "malformed":
				writeRuntimeTestFile(t, metadata, "{broken")
			case "directory":
				if err := os.Mkdir(metadata, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "keep.json")
				writeRuntimeTestFile(t, outside, "user metadata")
				if err := os.Symlink(outside, metadata); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err == nil {
				t.Error("unsafe ownership metadata allowed runtime replacement")
			}
			checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "user binary")
			if scenario == "malformed" {
				checkRuntimeTestFile(t, metadata, "{broken")
			} else if scenario == "symlink" {
				checkRuntimeTestFile(t, metadata, "user metadata")
			}
		})
	}
}

func TestInstalledRuntimeManifestRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "runtime")
	manifest, err := buildRuntimeManifest(root, run.Runtime{Version: "first", Platform: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	file := manifest.Files[0]
	for _, test := range []struct {
		name, path, hash string
	}{
		{name: "traversal", path: "../outside", hash: file.Hash},
		{name: "invalid hash", path: file.Path, hash: "invalid"},
		{name: "metadata as runtime file", path: installedRuntimeManifestFileName, hash: file.Hash},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := manifest
			current.Files = []runtimeFileInfo{{Path: test.path, Size: file.Size, Hash: test.hash, Mode: file.Mode}}
			if err := writeRuntimeManifest(filepath.Join(root, installedRuntimeManifestFileName), current); err != nil {
				t.Fatal(err)
			}
			if _, err := loadInstalledRuntimeManifest(root); !errors.Is(err, os.ErrInvalid) {
				t.Errorf("installed manifest with %s = %v, want os.ErrInvalid", test.name, err)
			}
			checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "runtime")
		})
	}
}

type runtimeUpgradeCancellationContext struct {
	context.Context
	cancel context.CancelFunc
	binary string
}

func (c *runtimeUpgradeCancellationContext) Err() error {
	data, err := os.ReadFile(c.binary)
	if err == nil && string(data) == "second binary" {
		c.cancel()
	}
	return c.Context.Err()
}
