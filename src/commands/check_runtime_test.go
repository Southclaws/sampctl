package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
)

func TestCheckRuntimeRecoveryIsReadOnlyAndRepairsCommittedCleanup(t *testing.T) {
	t.Parallel()
	pkg, root, record := checkRuntimeFixture(t)
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	results, err := checkRuntime(t.Context(), pkg, checkOptions{Platform: "linux"})
	if err != nil || len(results) != 1 || results[0].Status != checkFailed || !results[0].CanFix {
		t.Fatalf("checkRuntime(pending cleanup) = %+v, %v, want one repairable failure", results, err)
	}
	after, err := os.ReadFile(record)
	if err != nil || string(after) != string(before) {
		t.Fatalf("checkRuntime(read only) recovery = %q, %v, want unchanged record", after, err)
	}
	results, err = checkRuntime(t.Context(), pkg, checkOptions{Platform: "linux", Fix: true})
	if err != nil || len(results) != 2 || results[0].Status != checkRepaired || results[1].Status != checkPassed {
		t.Fatalf("checkRuntime(fix committed cleanup) = %+v, %v, want verified repair and valid runtime", results, err)
	}
	if _, err := os.Lstat(record); !os.IsNotExist(err) {
		t.Errorf("checkRuntime(fix committed cleanup) recovery record error = %v, want removed record", err)
	}
	for _, name := range []string{"samp03svr", "server.cfg", "user.txt"} {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != "original "+name {
			t.Errorf("checkRuntime(fix committed cleanup) %s = %q, %v, want preserved contents", name, got, err)
		}
	}
	results, err = checkRuntime(t.Context(), pkg, checkOptions{Platform: "linux", Fix: true})
	if err != nil || len(results) != 1 || results[0].Status != checkPassed || results[0].RepairAttempted {
		t.Errorf("checkRuntime(repeated fix) = %+v, %v, want valid runtime without another repair", results, err)
	}
}

func TestCheckRuntimeRefusesChangedFilesDuringRecovery(t *testing.T) {
	t.Parallel()
	pkg, root, record := checkRuntimeFixture(t)
	if err := os.WriteFile(filepath.Join(root, "samp03svr"), []byte("user changes"), 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	results, err := checkRuntime(t.Context(), pkg, checkOptions{Platform: "linux", Fix: true})
	if err != nil || len(results) != 1 || results[0].Status != checkFailed || results[0].CanFix || results[0].RepairAttempted {
		t.Fatalf("checkRuntime(changed committed runtime) = %+v, %v, want manual inspection", results, err)
	}
	after, err := os.ReadFile(record)
	if err != nil || string(after) != string(before) {
		t.Errorf("checkRuntime(changed committed runtime) record = %q, %v, want preserved record", after, err)
	}
	got, err := os.ReadFile(filepath.Join(root, "samp03svr"))
	if err != nil || string(got) != "user changes" {
		t.Errorf("checkRuntime(changed committed runtime) binary = %q, %v, want user changes", got, err)
	}
}

func TestCheckRuntimeDoesNotCreateMissingInstallation(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "not-installed")
	pkg := pawnpackage.Package{LocalPath: t.TempDir(), RuntimeDir: root}
	results, err := checkRuntime(t.Context(), pkg, checkOptions{Fix: true, Platform: "linux"})
	if err != nil || len(results) != 1 || results[0].Status != checkSkipped {
		t.Errorf("checkRuntime(no installation) = %+v, %v, want skipped check", results, err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Errorf("checkRuntime(no installation) directory error = %v, want no created directory", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := checkRuntime(ctx, pkg, checkOptions{}); err != context.Canceled {
		t.Errorf("checkRuntime(cancelled) error = %v, want context.Canceled", err)
	}
}

func checkRuntimeFixture(t *testing.T) (pawnpackage.Package, string, string) {
	t.Helper()
	root := t.TempDir()
	pkg := pawnpackage.Package{LocalPath: t.TempDir(), RuntimeDir: root, Preset: "samp"}
	for _, name := range []string{"samp03svr", "server.cfg", "user.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("original "+name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	type fileRecord struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
		Hash string `json:"hash"`
		Mode uint32 `json:"mode"`
	}
	file := func(name string, content []byte, mode uint32) fileRecord {
		hash := sha256.Sum256(content)
		return fileRecord{Path: name, Size: int64(len(content)), Hash: hex.EncodeToString(hash[:]), Mode: mode}
	}
	current := struct {
		Version     string       `json:"version"`
		Platform    string       `json:"platform"`
		RuntimeType string       `json:"runtime_type"`
		Files       []fileRecord `json:"files"`
	}{"0.3.7", "linux", "samp", []fileRecord{file("samp03svr", []byte("original samp03svr"), 0o700)}}
	ownership, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sampctl-runtime.json"), ownership, 0o600); err != nil {
		t.Fatal(err)
	}
	stage := ".sampctl-runtime-operation-check-fixture"
	if err := os.Mkdir(filepath.Join(root, stage), 0o700); err != nil {
		t.Fatal(err)
	}
	journal := map[string]any{
		"format": 1, "phase": "committed", "stage": stage, "current": current,
		"files": []map[string]any{
			{"path": "samp03svr", "after": current.Files[0]},
			{"path": ".sampctl-runtime.json", "after": file(".sampctl-runtime.json", ownership, 0o600)},
		},
	}
	data, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, runtimepkg.RuntimeRecoveryFilename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return pkg, root, path
}
