package pkgcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyCleanupPreservesChangedOrUnknownBackupFiles(t *testing.T) {
	for _, tc := range []struct {
		name, relative, contents string
	}{
		{name: "changed backup file", relative: "keep.inc", contents: "user backup changes"},
		{name: "unknown nested file", relative: "manual/notes.txt", contents: "user recovery notes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDependencyRecoveryFixture(t)
			cleanupFailure := errors.New("cleanup interrupted before removing files")
			fixture.pcx.dependencyInstalls.removeAll = func(string) error { return cleanupFailure }
			if err := fixture.pcx.finishDependencyOperation(fixture.state, nil); !errors.Is(err, cleanupFailure) {
				t.Fatalf("finishDependencyOperation(cleanup failure) = %v, want cleanup cause", err)
			}
			backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
			changed := filepath.Join(backup, filepath.FromSlash(tc.relative))
			if err := os.MkdirAll(filepath.Dir(changed), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(changed, []byte(tc.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			status, err := InspectDependencyRecovery(context.Background(), fixture.pcx.Package.LocalPath)
			if err == nil || !status.Committed || status.CanRecover {
				t.Errorf("InspectDependencyRecovery(%s) = %+v, %v; want committed state requiring manual recovery", tc.name, status, err)
			}
			if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
				t.Errorf("RecoverDependencyOperation(%s) = nil, want refusal", tc.name)
			}
			if got := string(readCrashRecoveryFile(t, changed)); got != tc.contents {
				t.Errorf("backup after cleanup retry = %q, want %q", got, tc.contents)
			}
			assertRecoveryEvidence(t, fixture, "replacement")
		})
	}
}

func TestDependencyCleanupRejectsInvalidInventory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*dependencyRecoveryStage)
	}{
		{name: "path traversal", mutate: func(stage *dependencyRecoveryStage) { stage.CleanupEntries[0].Path = "../outside" }},
		{name: "case alias", mutate: func(stage *dependencyRecoveryStage) {
			entry := stage.CleanupEntries[0]
			entry.Path = strings.ToUpper(entry.Path)
			stage.CleanupEntries = append(stage.CleanupEntries, entry)
		}},
		{name: "unsupported type", mutate: func(stage *dependencyRecoveryStage) { stage.CleanupEntries[0].Mode = uint32(os.ModeNamedPipe) }},
		{name: "invalid checksum", mutate: func(stage *dependencyRecoveryStage) { stage.CleanupEntries[0].Hash = "invalid" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDependencyRecoveryFixture(t)
			cleanupFailure := errors.New("cleanup interrupted")
			fixture.pcx.dependencyInstalls.removeAll = func(string) error { return cleanupFailure }
			if err := fixture.pcx.finishDependencyOperation(fixture.state, nil); !errors.Is(err, cleanupFailure) {
				t.Fatal(err)
			}
			tc.mutate(&fixture.state.journal.Stages[0])
			if err := fixture.state.journal.save(); err != nil {
				t.Fatal(err)
			}
			if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
				t.Errorf("RecoverDependencyOperation(%s inventory) = nil, want invalid metadata error", tc.name)
			}
			assertRecoveryEvidence(t, fixture, "replacement")
			backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
			if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "previous" {
				t.Errorf("backup with %s inventory = %q, want previous", tc.name, got)
			}
		})
	}
}

func TestDependencyCleanupDetectsSameSizeContentChanges(t *testing.T) {
	for _, tc := range []struct {
		name, resource string
		restored       bool
	}{
		{name: "committed target", resource: "target"},
		{name: "committed backup", resource: "backup"},
		{name: "restored target", resource: "target", restored: true},
		{name: "restored staged replacement", resource: "source", restored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDependencyRecoveryFixture(t)
			cleanupFailure := errors.New("cleanup interrupted")
			fixture.pcx.dependencyInstalls.removeAll = func(string) error { return cleanupFailure }
			var installationError error
			if tc.restored {
				installationError = errors.New("installation failed")
			}
			if err := fixture.pcx.finishDependencyOperation(fixture.state, installationError); !errors.Is(err, cleanupFailure) {
				t.Fatalf("finishDependencyOperation = %v, want cleanup failure", err)
			}
			root := fixture.target
			if tc.resource == "source" {
				root = fixture.source
			} else if tc.resource == "backup" {
				root = fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
			}
			path := filepath.Join(root, "keep.inc")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			changed := strings.Repeat("x", int(info.Size()))
			if err := os.WriteFile(path, []byte(changed), info.Mode().Perm()); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			status, err := InspectDependencyRecovery(context.Background(), fixture.pcx.Package.LocalPath)
			if err == nil || status.CanRecover {
				t.Errorf("InspectDependencyRecovery(%s) = %+v, %v, want refusal", tc.name, status, err)
			}
			if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
				t.Errorf("RecoverDependencyOperation(%s) = nil, want refusal", tc.name)
			}
			if got := string(readCrashRecoveryFile(t, path)); got != changed {
				t.Errorf("changed %s after recovery = %q, want %q", tc.resource, got, changed)
			}
			if _, err := os.Lstat(filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)); err != nil {
				t.Errorf("recovery lost its pending record: %v", err)
			}
		})
	}
}

func TestDependencyCleanupRejectsBackupAfterRestoration(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	cleanupFailure := errors.New("cleanup interrupted")
	fixture.pcx.dependencyInstalls.removeAll = func(string) error { return cleanupFailure }
	if err := fixture.pcx.finishDependencyOperation(fixture.state, errors.New("installation failed")); !errors.Is(err, cleanupFailure) {
		t.Fatal(err)
	}
	journal := fixture.state.journal
	backup := journal.path(journal.Changes[0].Backup)
	writeCrashRecoveryContent(t, backup, "unexpected backup")
	entries, err := recoveryCleanupEntries(context.Background(), fixture.stage)
	if err != nil {
		t.Fatal(err)
	}
	journal.Stages[0].CleanupEntries = entries
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("RecoverDependencyOperation(restored with backup) = nil, want refusal")
	}
	assertRecoveryEvidence(t, fixture, "previous")
	if got := string(readCrashRecoveryFile(t, filepath.Join(backup, "keep.inc"))); got != "unexpected backup" {
		t.Errorf("backup after recovery = %q, want unexpected backup", got)
	}
}

func TestDependencyRecoveryRetainsSnapshotWhenRestoredManifestIsMissing(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	manifest := filepath.Join(fixture.pcx.Package.LocalPath, "pawn.json")
	cleanupFailure := errors.New("rollback cleanup interrupted")
	fixture.pcx.dependencyInstalls.removeAll = func(string) error { return cleanupFailure }
	if err := fixture.pcx.finishDependencyOperation(fixture.state, errors.New("installation failed")); !errors.Is(err, cleanupFailure) {
		t.Fatal(err)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
		t.Error("RecoverDependencyOperation(missing restored manifest) = nil, want refusal preserving evidence")
	}
	if got := string(readCrashRecoveryFile(t, filepath.Join(fixture.target, "keep.inc"))); got != "previous" {
		t.Errorf("dependency after cleanup retry = %q, want previous", got)
	}
	if _, err := os.Lstat(filepath.Join(fixture.pcx.Package.LocalPath, DependencyRecoveryFilename)); err != nil {
		t.Errorf("unsafe rollback cleanup lost recovery metadata: %v", err)
	}
}
