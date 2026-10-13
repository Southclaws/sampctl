package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestRuntimeUpgradeFailuresRestoreVerifiedInstallation(t *testing.T) {
	t.Parallel()
	for _, point := range []string{"prepared", "published:samp03svr", "before:obsolete", "published:obsolete", "before:" + installedRuntimeManifestFileName, "published:" + installedRuntimeManifestFileName} {
		t.Run(point, func(t *testing.T) {
			root, source, current, previous := prepareRuntimeUpgradeFixture(t)
			injected := errors.New("injected runtime failure")
			err := installRuntime(context.Background(), root, source, current, previous, runtimeInstallHooks{after: func(actual string) error {
				if actual == point {
					return injected
				}
				return nil
			}})
			var outcome *RuntimeInstallError
			if !errors.Is(err, injected) || !errors.As(err, &outcome) || !outcome.Restored || outcome.Committed || outcome.RecoveryPending {
				t.Fatalf("installRuntime(%s) = %+v, want restored outcome wrapping injected error", point, err)
			}
			if err := VerifyInstalledRuntime(root); err != nil {
				t.Errorf("VerifyInstalledRuntime(after %s) = %v", point, err)
			}
			checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "old binary")
			checkRuntimeTestFile(t, filepath.Join(root, "obsolete"), "old resource")
			checkRuntimeTestFile(t, filepath.Join(root, "server.cfg"), "user configuration")
			if _, err := os.Lstat(filepath.Join(root, "library")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("failed upgrade retained new resource: %v", err)
			}
			if err := installRuntime(context.Background(), root, source, current, previous, runtimeInstallHooks{}); err != nil {
				t.Errorf("retry installRuntime(%s) = %v", point, err)
			}
		})
	}
}

func TestRuntimeCommittedCleanupKeepsReplacement(t *testing.T) {
	t.Parallel()
	root, source, current, previous := prepareRuntimeUpgradeFixture(t)
	injected := errors.New("cleanup interrupted")
	err := installRuntime(context.Background(), root, source, current, previous, runtimeInstallHooks{after: func(point string) error {
		if point == "committed" {
			return injected
		}
		return nil
	}})
	var outcome *RuntimeInstallError
	if !errors.Is(err, injected) || !errors.As(err, &outcome) || !outcome.Committed || outcome.Restored || !outcome.RecoveryPending {
		t.Fatalf("installRuntime(cleanup failure) = %+v, want committed outcome with pending recovery", err)
	}
	status, err := InspectRuntimeRecovery(context.Background(), root)
	if err != nil || !status.Pending || !status.Committed || !status.CanRecover {
		t.Fatalf("InspectRuntimeRecovery(committed) = %+v, %v", status, err)
	}
	for range 2 {
		if err := RecoverRuntime(context.Background(), root); err != nil {
			t.Fatal(err)
		}
	}
	checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "new binary")
	if err := VerifyInstalledRuntime(root); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("RecoverRuntime retained journal: %v", err)
	}
}

func TestRuntimeRecoveryAfterProcessTermination(t *testing.T) {
	if os.Getenv("SAMPCTL_RUNTIME_INSTALL_CRASH") == "1" {
		root, source := os.Getenv("SAMPCTL_RUNTIME_INSTALL_ROOT"), os.Getenv("SAMPCTL_RUNTIME_INSTALL_SOURCE")
		previous, err := loadInstalledRuntimeManifest(root)
		if err != nil {
			t.Fatal(err)
		}
		current, err := readRuntimeManifest(runtimeManifestPath(source))
		if err != nil {
			t.Fatal(err)
		}
		lock, err := lockRuntime(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close() //nolint:errcheck
		err = installRuntime(context.Background(), root, source, current, previous, runtimeInstallHooks{after: func(point string) error {
			if point == os.Getenv("SAMPCTL_RUNTIME_INSTALL_POINT") {
				os.Exit(73)
			}
			return nil
		}})
		t.Fatalf("runtime operation did not terminate: %v", err)
	}
	for _, point := range []string{"prepared", "published:samp03svr", "published:obsolete", "published:" + installedRuntimeManifestFileName, "committed", "cleanup:old-000001", "cleanup:stage"} {
		t.Run(point, func(t *testing.T) {
			root, source, _, previous := prepareRuntimeUpgradeFixture(t)
			crashRuntimeUpgrade(t, root, source, point)
			status, err := InspectRuntimeRecovery(context.Background(), root)
			committed := point == "committed" || point == "cleanup:old-000001" || point == "cleanup:stage"
			if err != nil || !status.Pending || !status.CanRecover || status.Committed != committed {
				t.Fatalf("InspectRuntimeRecovery(%s) = %+v, %v", point, status, err)
			}
			for range 2 {
				if err := RecoverRuntime(context.Background(), root); err != nil {
					t.Fatalf("RecoverRuntime(%s) = %v", point, err)
				}
			}
			want := "old binary"
			if committed {
				want = "new binary"
			} else if err := verifyRuntimeManifest(*previous, root); err != nil {
				t.Errorf("recovered prior runtime: %v", err)
			}
			checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), want)
			checkRuntimeTestFile(t, filepath.Join(root, "server.cfg"), "user configuration")
		})
	}
}

func TestRuntimeRecoveryRejectsUnexpectedChanges(t *testing.T) {
	for _, scenario := range []string{"modified destination", "modified backup", "missing backup", "unknown stage file", "unsafe journal path", "changed ownership fingerprint", "required file removed from journal"} {
		t.Run(scenario, func(t *testing.T) {
			root, source, _, _ := prepareRuntimeUpgradeFixture(t)
			crashRuntimeUpgrade(t, root, source, "published:samp03svr")
			journal, err := readRuntimeJournal(root)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "modified destination":
				writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "user changes")
			case "modified backup":
				writeRuntimeTestFile(t, runtimeStageFile(root, journal, 1, "old"), "changed backup")
			case "missing backup":
				if err := os.Remove(runtimeStageFile(root, journal, 1, "old")); err != nil {
					t.Fatal(err)
				}
			case "unknown stage file":
				writeRuntimeTestFile(t, filepath.Join(root, journal.Stage, "user.txt"), "user content")
			case "unsafe journal path":
				journal.Stage = "../outside"
				if err := writeRuntimeJournal(root, journal); err != nil {
					t.Fatal(err)
				}
			case "changed ownership fingerprint":
				journal.Files[len(journal.Files)-1].After.Hash = journal.Files[1].Before.Hash
				if err := writeRuntimeJournal(root, journal); err != nil {
					t.Fatal(err)
				}
			case "required file removed from journal":
				journal.Files[1].After = nil
				if err := writeRuntimeJournal(root, journal); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filepath.Join(root, "samp03svr"))
			if err != nil {
				t.Fatal(err)
			}
			status, err := InspectRuntimeRecovery(context.Background(), root)
			if err == nil || !status.Pending || status.CanRecover {
				t.Errorf("InspectRuntimeRecovery(%s) = %+v, %v, want unsafe pending state", scenario, status, err)
			}
			if err := RecoverRuntime(context.Background(), root); err == nil {
				t.Errorf("RecoverRuntime(%s) = nil, want rejected recovery", scenario)
			}
			checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), string(before))
			if _, err := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename)); err != nil {
				t.Errorf("rejected recovery removed evidence: %v", err)
			}
		})
	}
}

func TestRuntimeRecoveryInspectionDoesNotCreateFiles(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "missing")
	status, err := InspectRuntimeRecovery(context.Background(), root)
	if err != nil || status.Pending {
		t.Errorf("InspectRuntimeRecovery(no installation) = %+v, %v", status, err)
	}
	if err := RecoverRuntime(context.Background(), root); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("inspection or no-op recovery created installation: %v", err)
	}
}

func TestRuntimeRecoveryInspectionReportsActiveUpdate(t *testing.T) {
	t.Parallel()
	root, source, _, _ := prepareRuntimeUpgradeFixture(t)
	crashRuntimeUpgrade(t, root, source, "prepared")
	lock, err := lockRuntime(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close() //nolint:errcheck
	status, err := InspectRuntimeRecovery(context.Background(), root)
	if err == nil || !status.Pending || !status.Active || status.CanRecover {
		t.Errorf("InspectRuntimeRecovery(active) = %+v, %v, want active pending update", status, err)
	}
}

func TestRuntimeOwnershipAdoptionRecoversPendingUpgrade(t *testing.T) {
	t.Parallel()
	root, source, _, previous := prepareRuntimeUpgradeFixture(t)
	crashRuntimeUpgrade(t, root, source, "published:samp03svr")
	files := make([]lockfile.LockedFileInfo, len(previous.Files))
	for index, file := range previous.Files {
		files[index] = lockfile.LockedFileInfo(file)
	}
	locked := &lockfile.LockedRuntime{Version: previous.Version, Platform: previous.Platform, RuntimeType: string(previous.RuntimeType), Files: files}
	adopted, err := AdoptRuntimeOwnership(context.Background(), root, locked)
	if err != nil || adopted {
		t.Errorf("AdoptRuntimeOwnership(pending upgrade) = %v, %v, want existing ownership retained after recovery", adopted, err)
	}
	checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "old binary")
	if err := VerifyInstalledRuntime(root); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(filepath.Join(root, RuntimeRecoveryFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ownership adoption left pending recovery: %v", err)
	}
}

func TestRuntimeMissingStagedFilePreservesPreviousInstallation(t *testing.T) {
	t.Parallel()
	root, source, current, previous := prepareRuntimeUpgradeFixture(t)
	err := installRuntime(context.Background(), root, source, current, previous, runtimeInstallHooks{after: func(point string) error {
		if point != "prepared" {
			return nil
		}
		journal, err := readRuntimeJournal(root)
		if err != nil {
			return err
		}
		return os.Remove(runtimeStageFile(root, journal, 0, "new"))
	}})
	var outcome *RuntimeInstallError
	if !errors.As(err, &outcome) || outcome.Committed || outcome.RecoveryPending || !outcome.Restored {
		t.Fatalf("installRuntime(missing staged file) = %+v, want prior runtime restored", err)
	}
	if err := verifyRuntimeManifest(*previous, root); err != nil {
		t.Errorf("prior runtime after failed publication: %v", err)
	}
}

func TestEnsureRuntimePreservesModifiedManagedFile(t *testing.T) {
	t.Parallel()
	root, _, _, _ := prepareRuntimeUpgradeFixture(t)
	cache := t.TempDir()
	cfg := run.Runtime{Version: "new", Platform: "linux", WorkingDir: root}
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "new binary"})
	writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "user binary")
	if _, err := EnsureBinariesContext(context.Background(), cache, cfg); !errors.Is(err, os.ErrExist) {
		t.Errorf("EnsureBinariesContext(modified owned file) = %v, want os.ErrExist", err)
	}
	checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "user binary")
}

func TestRuntimePreparationFailureReportsPreservedInstallation(t *testing.T) {
	t.Parallel()
	root, _, _, _ := prepareRuntimeUpgradeFixture(t)
	cfg := run.Runtime{Version: "../invalid", Platform: "linux", WorkingDir: root}
	_, err := EnsureBinariesContext(context.Background(), t.TempDir(), cfg)
	var outcome *RuntimeInstallError
	if !errors.As(err, &outcome) || !outcome.Restored || outcome.Committed || outcome.RecoveryPending {
		t.Fatalf("EnsureBinariesContext(preparation failure) = %+v, want previous installation preserved", err)
	}
	checkRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "old binary")
	if err := VerifyInstalledRuntime(root); err != nil {
		t.Error(err)
	}
}

func TestConcurrentRuntimeInstallationAndCancellation(t *testing.T) {
	t.Parallel()
	root, cache := t.TempDir(), t.TempDir()
	cfg := run.Runtime{Version: "new", Platform: "linux", WorkingDir: root}
	seedValidatedRuntimeStage(t, cache, cfg, map[string]string{"samp03svr": "new binary", "library": "resource"})
	lock, err := lockRuntime(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := EnsureBinariesContext(ctx, cache, cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("EnsureBinariesContext(waiting for lock) = %v, want deadline exceeded", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan error, 4)
	for range 4 {
		workers.Go(func() {
			_, err := EnsureBinariesContext(context.Background(), cache, cfg)
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
	if err := VerifyInstalledRuntime(root); err != nil {
		t.Error(err)
	}
}

func TestSharedRuntimeStageLockProtectsMultipleProjects(t *testing.T) {
	t.Parallel()
	cache, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	platform := currentTestPlatform()
	seedRuntimeCacheFixture(t, cache, "0.3.7", platform)
	cfg := run.Runtime{Version: "0.3.7", Platform: platform, WorkingDir: first}
	lock, err := lockRuntimeStage(context.Background(), cache, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := EnsureBinariesContext(ctx, cache, cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("EnsureBinariesContext(shared stage busy) = %v, want deadline exceeded", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for _, root := range []string{first, second} {
		workers.Go(func() {
			projectCfg := cfg
			projectCfg.WorkingDir = root
			_, err := EnsureBinariesContext(context.Background(), cache, projectCfg)
			results <- err
		})
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
	for _, root := range []string{first, second} {
		if err := VerifyInstalledRuntime(root); err != nil {
			t.Errorf("VerifyInstalledRuntime(%s) = %v", root, err)
		}
	}
}

func prepareRuntimeUpgradeFixture(t *testing.T) (root, source string, current runtimeManifest, previous *runtimeManifest) {
	t.Helper()
	root, source = t.TempDir(), t.TempDir()
	writeRuntimeTestFile(t, filepath.Join(root, "samp03svr"), "old binary")
	writeRuntimeTestFile(t, filepath.Join(root, "obsolete"), "old resource")
	old, err := buildRuntimeManifest(root, run.Runtime{Version: "old", Platform: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeInstalledRuntimeManifest(context.Background(), root, old); err != nil {
		t.Fatal(err)
	}
	writeRuntimeTestFile(t, filepath.Join(root, "server.cfg"), "user configuration")
	writeRuntimeTestFile(t, filepath.Join(source, "samp03svr"), "new binary")
	writeRuntimeTestFile(t, filepath.Join(source, "library"), "new resource")
	current, err = buildRuntimeManifest(source, run.Runtime{Version: "new", Platform: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeManifest(runtimeManifestPath(source), current); err != nil {
		t.Fatal(err)
	}
	return root, source, current, &old
}

func crashRuntimeUpgrade(t *testing.T, root, source, point string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRuntimeRecoveryAfterProcessTermination$")
	command.Env = append(os.Environ(),
		"SAMPCTL_RUNTIME_INSTALL_CRASH=1",
		"SAMPCTL_RUNTIME_INSTALL_ROOT="+root,
		"SAMPCTL_RUNTIME_INSTALL_SOURCE="+source,
		"SAMPCTL_RUNTIME_INSTALL_POINT="+point,
	)
	output, err := command.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 73 {
		t.Fatalf("runtime crash at %s = %v, output %q, want exit 73", point, err, output)
	}
}
