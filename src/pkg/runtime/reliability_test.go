package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestEnsureBinariesPreservesInstallationOnUnsafeDestination(t *testing.T) {
	cfg := run.Runtime{Version: "replacement", Platform: "linux", WorkingDir: t.TempDir()}
	cache := t.TempDir()
	stage := seedValidatedRuntimeStage(t, cache, cfg, map[string]string{
		"samp03svr": "replacement binary",
		"z-library": "replacement library",
	})
	writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "samp03svr"), "previous binary")
	writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "obsolete"), "previous library")
	previous, err := buildRuntimeManifest(cfg.WorkingDir, run.Runtime{Version: "previous", Platform: cfg.Platform})
	if err != nil {
		t.Fatal(err)
	}
	saveRuntimeTestLockfile(t, cfg.WorkingDir, previous)
	if err := os.Mkdir(filepath.Join(cfg.WorkingDir, "z-library"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("EnsureBinariesContext(unsafe destination) = %v, want os.ErrInvalid", err)
	}
	checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "samp03svr"), "previous binary")
	checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "obsolete"), "previous library")
	checkRuntimeTestFile(t, filepath.Join(stage, "samp03svr"), "replacement binary")
}

func TestEnsureBinariesPreservesUntrackedExternalInstallation(t *testing.T) {
	cfg := run.Runtime{Version: "replacement", Platform: "linux", WorkingDir: t.TempDir()}
	cache := t.TempDir()
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "replacement"})
	writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "samp03svr"), "user installation")
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); !errors.Is(err, os.ErrExist) {
		t.Errorf("EnsureBinariesContext(untracked installation) = %v, want os.ErrExist", err)
	}
	checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "samp03svr"), "user installation")
}

func TestRuntimeCacheRebuildFailurePreservesPreviousFiles(t *testing.T) {
	cache := t.TempDir()
	platform := currentTestPlatform()
	seedRuntimeCacheFixture(t, cache, "0.3.7", platform)
	cfg := run.Runtime{Version: "0.3.7", Platform: platform, WorkingDir: t.TempDir()}
	stage := seedValidatedRuntimeStage(t, cache, cfg, map[string]string{expectedRuntimeBinary(platform): "previous cache"})
	writeRuntimeTestFile(t, runtimeManifestPath(stage), "invalid metadata")
	hr, err := infraresource.NewHTTPFileResource(offlineRuntimeURL(platform, cfg.Version), cfg.Version, infraresource.ResourceTypeServerBinary)
	if err != nil {
		t.Fatal(err)
	}
	hr.SetCacheDir(cache)
	hr.SetCacheTTL(0)
	hit, archive := hr.Cached(cfg.Version)
	if !hit {
		t.Fatal("seeded runtime archive is missing")
	}
	writeRuntimeTestFile(t, archive, "incomplete archive")
	if _, _, err := ensureStagedRuntime(context.Background(), cache, cfg); err == nil {
		t.Error("ensureStagedRuntime(incomplete archive) = nil, want extraction failure")
	}
	checkRuntimeTestFile(t, filepath.Join(stage, expectedRuntimeBinary(platform)), "previous cache")
}

func TestRuntimeCopyCancellationPreservesExistingFile(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	writeRuntimeTestFile(t, source, strings.Repeat("new", 10000))
	writeRuntimeTestFile(t, target, "previous")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := copyFileWithModeContext(ctx, source, target, 0o755); !errors.Is(err, context.Canceled) {
		t.Errorf("copyFileWithModeContext(canceled) = %v, want context.Canceled", err)
	}
	checkRuntimeTestFile(t, target, "previous")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("copyFileWithModeContext(canceled) left %d entries, want two original files", len(entries))
	}
}

func TestRuntimeCopyProcessInterruption(t *testing.T) {
	if os.Getenv("SAMPCTL_RUNTIME_COPY_CRASH") == "1" {
		ctx := &runtimeExitContext{Context: context.Background()}
		if err := copyFileWithModeContext(ctx, os.Getenv("SAMPCTL_RUNTIME_COPY_SOURCE"), os.Getenv("SAMPCTL_RUNTIME_COPY_TARGET"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Fatal("runtime copy completed without simulating termination")
	}
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	payload := strings.Repeat("replacement", 10000)
	writeRuntimeTestFile(t, source, payload)
	writeRuntimeTestFile(t, target, "previous valid binary")
	command := exec.Command(os.Args[0], "-test.run=^TestRuntimeCopyProcessInterruption$")
	command.Env = append(os.Environ(),
		"SAMPCTL_RUNTIME_COPY_CRASH=1",
		"SAMPCTL_RUNTIME_COPY_SOURCE="+source,
		"SAMPCTL_RUNTIME_COPY_TARGET="+target,
	)
	output, err := command.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 72 {
		t.Fatalf("interrupted runtime copy = %v, output %q, want exit 72", err, output)
	}
	pending, err := filepath.Glob(filepath.Join(root, ".target.*"))
	if err != nil || len(pending) != 1 {
		t.Fatalf("interrupted runtime copy left %v, error %v, want one temporary output", pending, err)
	}
	info, err := os.Stat(pending[0])
	if err != nil || info.Size() == 0 {
		t.Fatalf("interrupted runtime copy temporary output = %v, error %v, want nonempty file", info, err)
	}
	checkRuntimeTestFile(t, target, "previous valid binary")
	if err := copyFileWithModeContext(context.Background(), source, target, 0o755); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, target, payload)
}

type runtimeExitContext struct {
	context.Context
	calls int
}

func (c *runtimeExitContext) Err() error {
	c.calls++
	if c.calls == 4 {
		// Terminate after the first buffer reaches the temporary output file.
		os.Exit(72)
	}
	return nil
}

func TestEnsureBinariesPreservesModifiedObsoleteFiles(t *testing.T) {
	cfg := run.Runtime{Version: "replacement", Platform: "linux", WorkingDir: t.TempDir()}
	cache := t.TempDir()
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "replacement"})
	writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "samp03svr"), "previous")
	writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "obsolete"), "original")
	previous, err := buildRuntimeManifest(cfg.WorkingDir, run.Runtime{Version: "previous", Platform: cfg.Platform})
	if err != nil {
		t.Fatal(err)
	}
	saveRuntimeTestLockfile(t, cfg.WorkingDir, previous)
	writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "obsolete"), "user changes")
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "samp03svr"), "replacement")
	checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "obsolete"), "user changes")
}

func TestEnsureBinariesNativeRuntimeVariants(t *testing.T) {
	for _, tt := range []struct {
		name, version, platform, binary string
		runtimeType                     run.RuntimeType
	}{
		{name: "samp/linux", version: "0.3.7", platform: "linux", binary: "samp03svr", runtimeType: run.RuntimeTypeSAMP},
		{name: "samp/windows", version: "0.3.7", platform: "windows", binary: "samp-server.exe", runtimeType: run.RuntimeTypeSAMP},
		{name: "openmp/linux", version: "openmp", platform: "linux", binary: "omp-server", runtimeType: run.RuntimeTypeOpenMP},
		{name: "openmp/windows", version: "openmp", platform: "windows", binary: "omp-server.exe", runtimeType: run.RuntimeTypeOpenMP},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := run.Runtime{Version: tt.version, Platform: tt.platform, RuntimeType: tt.runtimeType, WorkingDir: t.TempDir()}
			cache := t.TempDir()
			seedValidatedRuntimeStage(t, cache, cfg, map[string]string{tt.binary: "runtime"})
			writeRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "server.cfg"), "user configuration")
			info, err := EnsureBinariesContext(context.Background(), cache, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if info.RuntimeType != string(tt.runtimeType) {
				t.Errorf("EnsureBinariesContext(%s).RuntimeType = %q, want %q", tt.name, info.RuntimeType, tt.runtimeType)
			}
			checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, tt.binary), "runtime")
			checkRuntimeTestFile(t, filepath.Join(cfg.WorkingDir, "server.cfg"), "user configuration")
		})
	}
}

func TestPluginExtractionFailurePreservesInstalledFiles(t *testing.T) {
	root := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(root, "plugins", "first.so"), "previous first")
	writeRuntimeTestFile(t, filepath.Join(root, "plugins", "second.so"), "previous second")
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	if err := w.WriteHeader(&tar.Header{Name: "first.so", Size: 3, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(&tar.Header{Name: "second.so", Size: 100, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("incomplete")); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "incomplete.tar.gz")
	if err := os.WriteFile(filename, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	request := EnsureVersionedPluginRequest{Context: context.Background(), Dir: root}
	if _, err := extractPluginArchive(request, filename, ".gz", map[string]string{
		"first.so":  "plugins/first.so",
		"second.so": "plugins/second.so",
	}); err == nil {
		t.Error("extractPluginArchive(truncated archive) = nil, want extraction error")
	}
	checkRuntimeTestFile(t, filepath.Join(root, "plugins", "first.so"), "previous first")
	checkRuntimeTestFile(t, filepath.Join(root, "plugins", "second.so"), "previous second")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "plugins" {
		t.Errorf("extractPluginArchive(truncated archive) left %v, want only plugins directory", entries)
	}
}

func TestPluginExtractionPreservesIgnoredAndExplicitResourceFiles(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(root, "plugins", "user.so"), "user plugin")
	filename := filepath.Join(t.TempDir(), "plugins.zip")
	createRuntimeZipArchive(t, filename, map[string]string{
		"user.so":   "replacement plugin",
		"extra.cfg": "extra configuration",
	})
	request := EnsureVersionedPluginRequest{Context: context.Background(), Dir: root, IgnorePatterns: []string{"user.so"}}
	target := filepath.Join(external, "configured.cfg")
	result, err := extractPluginArchive(request, filename, ".zip", map[string]string{
		"user.so":   "plugins/user.so",
		"extra.cfg": target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := result["user.so"]; exists {
		t.Error("extractPluginArchive(ignored plugin) returned user.so, want ignored file omitted")
	}
	checkRuntimeTestFile(t, filepath.Join(root, "plugins", "user.so"), "user plugin")
	checkRuntimeTestFile(t, target, "extra configuration")
}

func TestRuntimeManifestRejectsSymlinksAndTraversal(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(external, "file"), "user file")
	if err := os.Symlink(external, filepath.Join(root, "linked")); err != nil {
		t.Skipf("creating symlink: %v", err)
	}
	for _, relative := range []string{"linked/file", "../outside", filepath.Join(external, "file")} {
		manifest := runtimeManifest{Files: []runtimeFileInfo{{Path: relative, Size: 9}}}
		if err := verifyRuntimeManifest(manifest, root); !errors.Is(err, os.ErrInvalid) {
			t.Errorf("verifyRuntimeManifest(%q) = %v, want os.ErrInvalid", relative, err)
		}
		if err := removeRuntimeFiles(manifest, root); !errors.Is(err, os.ErrInvalid) {
			t.Errorf("removeRuntimeFiles(%q) = %v, want os.ErrInvalid", relative, err)
		}
	}
	checkRuntimeTestFile(t, filepath.Join(external, "file"), "user file")
}

func TestRuntimePackageRequiresAnExecutableServer(t *testing.T) {
	root := t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(root, "readme.txt"), "documentation")
	cfg := run.Runtime{Version: "0.3.7", Platform: "linux"}
	manifest, err := buildRuntimeManifest(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimePackage(manifest, root); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("validateRuntimePackage(no server) = %v, want os.ErrNotExist", err)
	}
	writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "")
	manifest, err = buildRuntimeManifest(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimePackage(manifest, root); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("validateRuntimePackage(empty server) = %v, want os.ErrInvalid", err)
	}
}

func seedValidatedRuntimeStage(t *testing.T, cache string, cfg run.Runtime, files map[string]string) string {
	t.Helper()
	stage := filepath.Join(cache, runtimeStagingDir, cfg.Platform, cfg.Version)
	for relative, data := range files {
		writeRuntimeTestFile(t, filepath.Join(stage, relative), data)
	}
	manifest, err := buildRuntimeManifest(stage, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeManifest(runtimeManifestPath(stage), manifest); err != nil {
		t.Fatal(err)
	}
	return stage
}

func writeRuntimeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
		t.Fatal(err)
	}
}

func checkRuntimeTestFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("ReadFile(%s) = %q, want %q", path, got, want)
	}
}

func saveRuntimeTestLockfile(t *testing.T, root string, manifest runtimeManifest) {
	t.Helper()
	lf := lockfile.New("test")
	files := make([]lockfile.LockedFileInfo, len(manifest.Files))
	for index, file := range manifest.Files {
		files[index] = lockfile.LockedFileInfo(file)
	}
	lf.SetRuntime(manifest.Version, manifest.Platform, string(manifest.RuntimeType), files)
	if err := lockfile.Save(root, lf); err != nil {
		t.Fatal(err)
	}
}
