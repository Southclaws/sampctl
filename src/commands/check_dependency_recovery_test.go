package commands

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestCheckDependencyRecoveryReadOnlyAndRepair(t *testing.T) {
	t.Parallel()
	root, stage := newCheckRecoveryFixture(t, "active")
	manifest := filepath.Join(root, "pawn.json")
	journal := filepath.Join(root, pkgcontext.DependencyRecoveryFilename)
	beforeManifest := readCheckRecoveryFixture(t, manifest)
	beforeJournal := readCheckRecoveryFixture(t, journal)
	results, state, err := checkDependencyRecovery(context.Background(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != checkFailed || !results[0].CanFix || !state.Pending {
		t.Fatalf("read-only recovery results = %+v, state = %+v", results, state)
	}
	if results[0].RepairAttempted {
		t.Error("read-only check attempted recovery")
	}
	if !bytes.Equal(readCheckRecoveryFixture(t, manifest), beforeManifest) || !bytes.Equal(readCheckRecoveryFixture(t, journal), beforeJournal) {
		t.Error("read-only recovery check changed project files")
	}
	if _, err := os.Lstat(filepath.Join(root, ".sampctl-dependencies.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("read-only check created a lock file: %v", err)
	}
	results, state, err = checkDependencyRecovery(context.Background(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != checkRepaired || !results[0].RepairVerified || state.Pending {
		t.Fatalf("repair results = %+v, state = %+v", results, state)
	}
	if got := string(readCheckRecoveryFixture(t, manifest)); got != `{"user":"before"}` {
		t.Errorf("manifest after recovery = %s, want recorded snapshot", got)
	}
	for _, path := range []string{journal, stage} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("completed recovery left %s: %v", path, err)
		}
	}
	results, state, err = checkDependencyRecovery(context.Background(), root, true)
	if err != nil || len(results) != 0 || state.Pending {
		t.Errorf("repeated recovery = %+v, state = %+v, error = %v", results, state, err)
	}
}

func TestRunChecksRepairsMissingManifestBeforeProjectChecks(t *testing.T) {
	t.Parallel()
	root, _ := newCheckRecoveryFixture(t, "active")
	if err := os.Remove(filepath.Join(root, "pawn.json")); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "nested")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	results, err := runChecks(context.Background(), checkOptions{Dir: child, CacheDir: t.TempDir(), Platform: "linux", Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"project", "dependency recovery"} {
		result := findCheckRecoveryResult(t, results, name)
		want := checkPassed
		if name == "dependency recovery" {
			want = checkRepaired
		}
		if result.Status != want {
			t.Errorf("%s after recovery = %+v, want %s", name, result, want)
		}
	}
}

func TestCheckCommittedCleanupPreservesManifest(t *testing.T) {
	t.Parallel()
	root, stage := newCheckRecoveryFixture(t, "committed")
	manifest := filepath.Join(root, "pawn.json")
	before := readCheckRecoveryFixture(t, manifest)
	results, _, err := checkDependencyRecovery(context.Background(), root, false)
	if err != nil || len(results) != 1 {
		t.Fatalf("inspect committed cleanup = %+v, error = %v", results, err)
	}
	if !strings.Contains(results[0].Message, "changes saved") || !results[0].CanFix {
		t.Errorf("committed cleanup diagnostic = %+v", results[0])
	}
	results, _, err = checkDependencyRecovery(context.Background(), root, true)
	if err != nil || len(results) != 1 || !results[0].RepairVerified {
		t.Fatalf("repair committed cleanup = %+v, error = %v", results, err)
	}
	if !bytes.Equal(before, readCheckRecoveryFixture(t, manifest)) {
		t.Error("cleanup reverted the committed manifest")
	}
	if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup retained recorded stage: %v", err)
	}
}

func TestCheckDependencyRecoveryPreservesInvalidAndChangedState(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"invalid metadata", "changed manifest", "unexpected staged file"} {
		t.Run(scenario, func(t *testing.T) {
			root, stage := newCheckRecoveryFixture(t, "active")
			journal := filepath.Join(root, pkgcontext.DependencyRecoveryFilename)
			switch scenario {
			case "invalid metadata":
				writeCheckRecoveryFixture(t, journal, []byte("{broken"))
			case "changed manifest":
				writeCheckRecoveryFixture(t, filepath.Join(root, "pawn.json"), []byte(`{"user":"manual edit"}`))
			case "unexpected staged file":
				writeCheckRecoveryFixture(t, filepath.Join(stage, "unknown.inc"), []byte("keep"))
			}
			before := snapshotCheckDependency(t, root)
			results, _, err := checkDependencyRecovery(context.Background(), root, true)
			if err != nil || len(results) != 1 {
				t.Fatalf("check invalid recovery = %+v, error = %v", results, err)
			}
			if results[0].Status != checkFailed || results[0].CanFix || results[0].RepairAttempted || results[0].Err == nil {
				t.Errorf("invalid recovery diagnostic = %+v", results[0])
			}
			if got := snapshotCheckDependency(t, root); !maps.Equal(got, before) {
				t.Error("invalid recovery check changed project files")
			}
		})
	}
}

func TestRunChecksDefersDependencyChecksWhileOperationIsActive(t *testing.T) {
	t.Parallel()
	root, _ := newCheckRecoveryFixture(t, "active")
	lock := flock.New(filepath.Join(root, ".sampctl-dependencies.lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Unlock(); err != nil {
			t.Error(err)
		}
	})
	before := readCheckRecoveryFixture(t, filepath.Join(root, pkgcontext.DependencyRecoveryFilename))
	results, err := runChecks(context.Background(), checkOptions{Dir: root, CacheDir: t.TempDir(), Platform: "linux", Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	recovery := findCheckRecoveryResult(t, results, "dependency recovery")
	if recovery.Status != checkFailed || recovery.CanFix || recovery.RepairAttempted || !strings.Contains(recovery.Message, "running in this project") {
		t.Errorf("active operation diagnostic = %+v", recovery)
	}
	for _, name := range []string{"dependencies", "lockfile", "compiler"} {
		if result := findCheckRecoveryResult(t, results, name); result.Status != checkSkipped {
			t.Errorf("%s during active operation = %+v, want skipped", name, result)
		}
	}
	if !bytes.Equal(before, readCheckRecoveryFixture(t, filepath.Join(root, pkgcontext.DependencyRecoveryFilename))) {
		t.Error("check --fix changed an active recovery record")
	}
}

func TestCheckDependencyRecoveryCancellation(t *testing.T) {
	t.Parallel()
	root, _ := newCheckRecoveryFixture(t, "active")
	before := snapshotCheckDependency(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := checkDependencyRecovery(ctx, root, true)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled recovery = %v, want context.Canceled", err)
	}
	if got := snapshotCheckDependency(t, root); !maps.Equal(got, before) {
		t.Error("cancelled recovery changed project files")
	}
}

func TestCheckRecoveryStagesKeepUnknownDirectoriesVisible(t *testing.T) {
	t.Parallel()
	root, stage := newCheckRecoveryFixture(t, "active")
	unknown := filepath.Join(root, "dependencies", ".sampctl-stage-unknown")
	if err := os.Mkdir(unknown, 0o700); err != nil {
		t.Fatal(err)
	}
	results, err := runChecks(context.Background(), checkOptions{Dir: root, CacheDir: t.TempDir(), Platform: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	var staging int
	for _, result := range results {
		if result.Name == "dependency staging" {
			staging++
			if result.Path != unknown || result.Path == stage {
				t.Errorf("staging diagnostic = %+v, want unknown directory only", result)
			}
		}
	}
	if staging != 1 {
		t.Errorf("unknown staging diagnostics = %d, want 1", staging)
	}
}

func newCheckRecoveryFixture(t *testing.T, phase string) (string, string) {
	t.Helper()
	root := t.TempDir()
	stageName := "dependencies/.sampctl-stage-check-fixture"
	stage := filepath.Join(root, filepath.FromSlash(stageName))
	if err := os.MkdirAll(filepath.Join(stage, "dependency"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeCheckRecoveryFixture(t, filepath.Join(stage, "dependency", "keep.inc"), []byte("staged"))
	writeCheckRecoveryFixture(t, filepath.Join(stage, ".sampctl-owner"), []byte("check"))
	writeCheckRecoveryFixture(t, filepath.Join(root, "pawn.json"), []byte(`{"user":"after"}`))
	snapshot := []byte(`{"user":"before"}`)
	snapshotHash := fmt.Sprintf("%x", sha256.Sum256(append([]byte(fmt.Sprintf("pawn.json:%d:true:", 0o600)), snapshot...)))
	journal := map[string]any{
		"version": 1, "id": "check", "operation": "install", "phase": phase,
		"files":  []map[string]any{{"path": "pawn.json", "data": snapshot, "mode": 0o600, "exists": true, "hash": snapshotHash, "writes": [][]byte{[]byte(`{"user":"after"}`)}}},
		"stages": []map[string]any{{"path": stageName, "allowed": []string{"dependency"}}},
	}
	data, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	writeCheckRecoveryFixture(t, filepath.Join(root, pkgcontext.DependencyRecoveryFilename), data)
	return root, stage
}

func readCheckRecoveryFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeCheckRecoveryFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func findCheckRecoveryResult(t *testing.T, results []diagnostic, name string) diagnostic {
	t.Helper()
	for _, result := range results {
		if result.Name == name {
			return result
		}
	}
	t.Fatalf("missing %s diagnostic in %+v", name, results)
	return diagnostic{}
}
