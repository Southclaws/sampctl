package pkgcontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

type dependencyRecoveryFixture struct {
	pcx                   *PackageContext
	state                 *dependencyProjectState
	stage, source, target string
}

func newDependencyRecoveryFixture(t *testing.T) dependencyRecoveryFixture {
	t.Helper()
	pcx := lifecycleContext(t, "json")
	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")
	target := filepath.Join(pcx.Package.Vendor, "dependency")
	writeCrashRecoveryContent(t, target, "previous")
	state, err := pcx.beginDependencyOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := state.lock.Close(); err != nil {
			t.Errorf("close fixture project lock: %v", err)
		}
	})
	stage, err := pcx.dependencyInstalls.stage(pcx.Package.Vendor, "dependency")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stage, "dependency")
	writeCrashRecoveryContent(t, source, "replacement")
	if err := pcx.dependencyInstalls.promote(source, target); err != nil {
		t.Fatal(err)
	}
	return dependencyRecoveryFixture{pcx: pcx, state: state, stage: stage, source: source, target: target}
}

func releaseDependencyRecoveryFixture(t *testing.T, fixture dependencyRecoveryFixture) {
	t.Helper()
	if err := fixture.state.lock.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveryEvidence(t *testing.T, fixture dependencyRecoveryFixture, targetContent string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)); err != nil {
		t.Errorf("unresolved operation lost recovery metadata: %v", err)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != targetContent {
		t.Errorf("dependency contents = %q, want %q", got, targetContent)
	}
}

func TestDependencyRecoveryRejectsMalformedRecordsWithoutMutatingInstallations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*dependencyJournal)
	}{
		{name: "unsupported version", mutate: func(journal *dependencyJournal) { journal.Version++ }},
		{name: "unknown phase", mutate: func(journal *dependencyJournal) { journal.Phase = "unknown" }},
		{name: "target traversal", mutate: func(journal *dependencyJournal) { journal.Changes[0].Target = "dependencies/../../outside" }},
		{name: "backup traversal", mutate: func(journal *dependencyJournal) { journal.Changes[0].Backup = "../outside" }},
		{name: "unowned staging", mutate: func(journal *dependencyJournal) { journal.Stages[0].Path = "dependencies/.sampctl-stage-unknown" }},
		{name: "invalid fingerprint", mutate: func(journal *dependencyJournal) { journal.Changes[0].NewHash = "invalid" }},
		{name: "snapshot traversal", mutate: func(journal *dependencyJournal) { journal.Files[0].Path = "../pawn.json" }},
		{name: "cleanup before commit", mutate: func(journal *dependencyJournal) { journal.Stages[0].Cleanup = true }},
		{name: "owned stage as target", mutate: func(journal *dependencyJournal) {
			journal.Changes[0].Target = "dependencies/.SAMPCTL-STAGE-" + journal.ID + "-0"
		}},
		{name: "overlapping targets", mutate: func(journal *dependencyJournal) {
			change := journal.Changes[0]
			change.Target += "/child"
			change.Source += "-other"
			change.Backup += "-other"
			journal.Changes = append(journal.Changes, change)
		}},
		{name: "case alias targets", mutate: func(journal *dependencyJournal) {
			change := journal.Changes[0]
			change.Target = "dependencies/" + strings.ToUpper(filepath.Base(change.Target))
			change.Source += "-other"
			change.Backup += "-other"
			journal.Changes = append(journal.Changes, change)
		}},
		{name: "shared backup", mutate: func(journal *dependencyJournal) {
			change := journal.Changes[0]
			change.Target += "-other"
			change.Source += "-other"
			journal.Changes = append(journal.Changes, change)
		}},
		{name: "case alias source", mutate: func(journal *dependencyJournal) {
			change := journal.Changes[0]
			change.Target += "-other"
			change.Source = filepath.ToSlash(filepath.Join(filepath.Dir(change.Source), strings.ToUpper(filepath.Base(change.Source))))
			change.Backup += "-other"
			journal.Changes = append(journal.Changes, change)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDependencyRecoveryFixture(t)
			backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
			tc.mutate(fixture.state.journal)
			if err := fixture.state.journal.validate(); err == nil {
				t.Errorf("validation accepted malformed %s metadata", tc.name)
			}
			data, err := json.Marshal(fixture.state.journal)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			releaseDependencyRecoveryFixture(t, fixture)
			if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
				t.Fatal("recovery accepted malformed metadata")
			}
			assertRecoveryEvidence(t, fixture, "replacement")
			if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "previous" {
				t.Errorf("backup contents = %q, want previous", got)
			}
			if got := readCrashRecoveryFile(t, path); !bytes.Equal(got, data) {
				t.Error("recovery rewrote malformed metadata")
			}
		})
	}
}

func TestDependencyRecoveryRejectsCorruptJSON(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	path := filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)
	if err := os.WriteFile(path, []byte(`{"version":`), 0o600); err != nil {
		t.Fatal(err)
	}
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := InspectDependencyRecovery(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("inspection accepted corrupt JSON")
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("recovery accepted corrupt JSON")
	}
	assertRecoveryEvidence(t, fixture, "replacement")
}

func TestDependencyRecoveryPreservesChangedTargetAndBackup(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	writeCrashRecoveryContent(t, fixture.target, "user edit")
	releaseDependencyRecoveryFixture(t, fixture)
	status, err := InspectDependencyRecovery(context.Background(), fixture.pcx.Package.LocalPath)
	if err == nil || status.CanRecover {
		t.Errorf("inspect changed target = %+v, %v; want manual recovery", status, err)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("recovery overwrote a changed target")
	}
	assertRecoveryEvidence(t, fixture, "user edit")
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "previous" {
		t.Errorf("backup contents = %q, want previous", got)
	}
}

func TestDependencyRecoveryRefusesMissingBackup(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	if err := os.RemoveAll(backup); err != nil {
		t.Fatal(err)
	}
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("recovery succeeded without the previous installation")
	}
	assertRecoveryEvidence(t, fixture, "replacement")
}

func TestDependencyRecoveryRejectsSymlinkedBackup(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	outside := filepath.Join(t.TempDir(), "previous")
	if err := os.Rename(backup, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, backup); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("recovery followed a symlinked backup")
	}
	assertRecoveryEvidence(t, fixture, "replacement")
	if got := string(readCrashRecoveryFile(t, filepath.Join(outside, "keep.inc"))); got != "previous" {
		t.Errorf("external backup contents = %q, want previous", got)
	}
}

func TestDependencyRecoveryLeavesUnknownStagingDirectories(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	unknown := filepath.Join(fixture.pcx.Package.Vendor, ".sampctl-stage-unknown")
	writeCrashRecoveryContent(t, unknown, "unknown")
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Fatal(err)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(unknown, "keep.inc"))); got != "unknown" {
		t.Errorf("unknown staged files = %q, want unknown", got)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != "previous" {
		t.Errorf("recovered dependency = %q, want previous", got)
	}
}

func TestDependencyRecoveryPreservesUnexpectedOwnedStageEntry(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	unknown := filepath.Join(fixture.stage, "unknown")
	writeCrashRecoveryContent(t, unknown, "user data")
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("recovery deleted an unexpected stage entry")
	}
	assertRecoveryEvidence(t, fixture, "replacement")
	if got := string(readCrashRecoveryFile(t, filepath.Join(unknown, "keep.inc"))); got != "user data" {
		t.Errorf("unexpected staged contents = %q, want user data", got)
	}
}

func TestDependencyRecoveryRespectsActiveLockAndCancellation(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	path := filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)
	before := readCrashRecoveryFile(t, path)
	status, err := InspectDependencyRecovery(context.Background(), fixture.pcx.Package.LocalPath)
	if err != nil || !status.Active || status.CanRecover {
		t.Errorf("inspect active operation = %+v, %v; want active nonrecoverable status", status, err)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("recovery acquired an active project lock")
	}
	releaseDependencyRecoveryFixture(t, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RecoverDependencyOperation(ctx, fixture.pcx.Package.LocalPath); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled recovery = %v, want context.Canceled", err)
	}
	if got := readCrashRecoveryFile(t, path); !bytes.Equal(got, before) {
		t.Error("active or canceled recovery changed its metadata")
	}
	assertRecoveryEvidence(t, fixture, "replacement")
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Fatal(err)
	}
}

func TestDependencyRecoveryCancellationAfterRenameCanResume(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rename := func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		if from == fixture.target {
			cancel()
		}
		return nil
	}
	err := fixture.state.journal.recover(ctx, rename, os.RemoveAll)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("recovery canceled after rename = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)); err != nil {
		t.Errorf("canceled recovery lost metadata: %v", err)
	}
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Fatalf("resume recovery canceled after rename: %v", err)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != "previous" {
		t.Errorf("resumed recovery dependency = %q, want previous", got)
	}
}

func TestDependencyRecoveryCompletesPartialCommittedCleanup(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	fixture.pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dependency:2.0.0"}
	if err := fixture.pcx.writeDependencyDefinition(); err != nil {
		t.Fatal(err)
	}
	manifest := readCrashRecoveryFile(t, filepath.Join(fixture.pcx.Package.LocalPath, "pawn.json"))
	failure := errors.New("cleanup file remained open")
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	fixture.pcx.dependencyInstalls.removeAll = func(string) error {
		if err := os.Remove(filepath.Join(fixture.stage, ".sampctl-owner")); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(backup, "keep.inc")); err != nil {
			return err
		}
		return failure
	}
	err := fixture.pcx.finishDependencyOperation(fixture.state, nil)
	var operation *OperationError
	if !errors.Is(err, failure) || !errors.As(err, &operation) || operation.Outcome != OutcomeCommitted {
		t.Fatalf("partial cleanup error = %v, outcome = %+v; want preserved cause and committed outcome", err, operation)
	}
	assertRecoveryEvidence(t, fixture, "replacement")
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Fatalf("retry partial committed cleanup: %v", err)
	}
	if got := readCrashRecoveryFile(t, filepath.Join(fixture.pcx.Package.LocalPath, "pawn.json")); !bytes.Equal(got, manifest) {
		t.Error("cleanup retry reverted committed manifest changes")
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != "replacement" {
		t.Errorf("committed dependency after cleanup retry = %q, want replacement", got)
	}
	if _, err := os.Lstat(fixture.stage); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup retry retained its stage: %v", err)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Errorf("repeat completed cleanup: %v", err)
	}
}

func TestDependencyRecoveryRetainsBackupWhenCommittedTargetIsMissing(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	fixture.state.journal.Phase = "committed"
	if err := fixture.state.journal.save(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(fixture.target); err != nil {
		t.Fatal(err)
	}
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("cleanup succeeded despite a missing committed replacement")
	}
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "previous" {
		t.Errorf("remaining backup contents = %q, want previous", got)
	}
	if _, err := os.Stat(filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)); err != nil {
		t.Errorf("unsafe committed cleanup lost metadata: %v", err)
	}
}

func TestDependencyRecoveryPreservesBackupChangedBeforeCleanupIntent(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	fixture.state.journal.Phase = "committed"
	if err := fixture.state.journal.save(); err != nil {
		t.Fatal(err)
	}
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	writeCrashRecoveryContent(t, backup, "user backup edit")
	releaseDependencyRecoveryFixture(t, fixture)
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("cleanup deleted a backup changed before cleanup began")
	}
	assertRecoveryEvidence(t, fixture, "replacement")
	if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "user backup edit" {
		t.Errorf("changed backup contents = %q, want user backup edit", got)
	}
}

func TestDependencyRecoveryReportsIncompleteRollbackAndCanRetry(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	operationFailure := errors.New("dependency preparation failed")
	rollbackFailure := errors.New("backup temporarily locked")
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	fixture.pcx.dependencyInstalls.rename = func(from, to string) error {
		if from == backup {
			return rollbackFailure
		}
		return os.Rename(from, to)
	}
	err := fixture.pcx.finishDependencyOperation(fixture.state, operationFailure)
	var operation *OperationError
	if !errors.Is(err, operationFailure) || !errors.Is(err, rollbackFailure) || !errors.As(err, &operation) || operation.Outcome != OutcomeRollbackIncomplete {
		t.Fatalf("rollback error = %v, outcome = %+v; want both causes and incomplete outcome", err, operation)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "previous" {
		t.Errorf("failed rollback backup = %q, want previous", got)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Fatalf("retry incomplete rollback: %v", err)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != "previous" {
		t.Errorf("recovered installation = %q, want previous", got)
	}
}

func TestDependencyRecoveryReportsRestoredProjectWithPendingCleanup(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	manifestPath := filepath.Join(fixture.pcx.Package.LocalPath, "pawn.json")
	originalManifest := readCrashRecoveryFile(t, manifestPath)
	fixture.pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dependency:2.0.0"}
	if err := fixture.pcx.writeDependencyDefinition(); err != nil {
		t.Fatal(err)
	}
	operationFailure := errors.New("dependency preparation failed")
	cleanupFailure := errors.New("staging file remained open")
	fixture.pcx.dependencyInstalls.removeAll = func(string) error { return cleanupFailure }
	err := fixture.pcx.finishDependencyOperation(fixture.state, operationFailure)
	var operation *OperationError
	if !errors.Is(err, operationFailure) || !errors.Is(err, cleanupFailure) || !errors.As(err, &operation) || operation.Outcome != OutcomeRolledBack {
		t.Fatalf("restored cleanup error = %v, outcome = %+v; want both causes and rolled-back outcome", err, operation)
	}
	assertRecoveryEvidence(t, fixture, "previous")
	if got := readCrashRecoveryFile(t, manifestPath); !bytes.Equal(got, originalManifest) {
		t.Error("failed operation did not restore the original manifest")
	}
	status, err := InspectDependencyRecovery(context.Background(), fixture.pcx.Package.LocalPath)
	if err != nil || !status.Restored || status.Committed || !status.CanRecover {
		t.Errorf("inspect restored pending cleanup = %+v, %v; want restored recoverable status", status, err)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err != nil {
		t.Fatal(err)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != "previous" {
		t.Errorf("cleanup changed the restored dependency to %q, want previous", got)
	}
}

func TestDependencyPromotionCrossFilesystemFailureKeepsPrevious(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")
	target := filepath.Join(pcx.Package.Vendor, "dependency")
	writeCrashRecoveryContent(t, target, "previous")
	state, err := pcx.beginDependencyOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := state.lock.Close(); err != nil {
			t.Errorf("close fixture project lock: %v", err)
		}
	})
	stage, err := pcx.dependencyInstalls.stage(pcx.Package.Vendor, "dependency")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stage, "dependency")
	writeCrashRecoveryContent(t, source, "replacement")
	pcx.dependencyInstalls.rename = func(string, string) error { return syscall.EXDEV }
	err = pcx.dependencyInstalls.promote(source, target)
	if !errors.Is(err, syscall.EXDEV) {
		t.Fatalf("cross-filesystem promotion = %v, want EXDEV", err)
	}
	err = pcx.finishDependencyOperation(state, err)
	var operation *OperationError
	if !errors.Is(err, syscall.EXDEV) || !errors.As(err, &operation) || operation.Outcome != OutcomeRolledBack {
		t.Errorf("cross-filesystem operation error = %v, outcome = %+v; want EXDEV and rolled-back outcome", err, operation)
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(target, "keep.inc"))); got != "previous" {
		t.Errorf("cross-filesystem failure changed dependency to %q, want previous", got)
	}
	if _, err := os.Stat(filepath.Join(pcx.Package.LocalPath, DependencyRecoveryFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("completed rollback retained recovery metadata: %v", err)
	}
}

func TestDependencyRecoveryRunsBeforeNextEnsure(t *testing.T) {
	for _, entrypoint := range []string{"project", "package"} {
		t.Run(entrypoint, func(t *testing.T) {
			pcx := lifecycleContext(t, "json")
			meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "1.0.0"}
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0"})
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep:1.0.0"}
			if err := pcx.Package.WriteDefinition(); err != nil {
				t.Fatal(err)
			}
			if err := pcx.InitLockfileResolver("test"); err != nil {
				t.Fatal(err)
			}
			if _, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{}); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(pcx.Package.Vendor, "dep")
			original := readCrashRecoveryFile(t, filepath.Join(target, "dep.pwn"))
			state, err := pcx.beginDependencyOperation(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := state.lock.Close(); err != nil {
					t.Errorf("close interrupted operation lock: %v", err)
				}
			})
			stage, err := pcx.dependencyInstalls.stage(pcx.Package.Vendor, "dep")
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(stage, "dep")
			if err := stagePreviousDependency(context.Background(), target, source); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "dep.pwn"), []byte("interrupted replacement"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := pcx.dependencyInstalls.promote(source, target); err != nil {
				t.Fatal(err)
			}
			if err := state.lock.Unlock(); err != nil {
				t.Fatal(err)
			}
			fresh, err := LoadPackageContext(NewPackageContextOptions{Dir: pcx.Package.LocalPath, Parent: true, CacheDir: pcx.CacheDir, Platform: "linux", RuntimeProv: &fakeRuntimeProvisioner{}})
			if err != nil {
				t.Fatal(err)
			}
			if err := fresh.InitLockfileResolver("test"); err != nil {
				t.Fatal(err)
			}
			if entrypoint == "project" {
				_, err = fresh.EnsureProject(context.Background(), DependencyUpdateRequest{})
			} else {
				err = fresh.EnsurePackage(meta, false)
			}
			if err != nil {
				t.Fatalf("next %s ensure: %v", entrypoint, err)
			}
			if got := readCrashRecoveryFile(t, filepath.Join(target, "dep.pwn")); !bytes.Equal(got, original) {
				t.Errorf("next %s ensure retained interrupted replacement", entrypoint)
			}
			if _, err := os.Stat(filepath.Join(pcx.Package.LocalPath, DependencyRecoveryFilename)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("next %s ensure retained recovery metadata: %v", entrypoint, err)
			}
			if _, err := lockfile.Load(pcx.Package.LocalPath); err != nil {
				t.Errorf("next %s ensure corrupted lockfile: %v", entrypoint, err)
			}
		})
	}
}
