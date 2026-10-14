package pkgcontext

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var recoveryBenchmarkWorkloads = []struct {
	name        string
	files, size int
}{
	{name: "small_files", files: 2048, size: 512},
	{name: "large_files", files: 4, size: 8 << 20},
}

func BenchmarkDependencyRecoveryFingerprint(b *testing.B) {
	ctx := context.Background()
	for _, workload := range recoveryBenchmarkWorkloads {
		b.Run(workload.name, func(b *testing.B) {
			root := b.TempDir()
			writeRecoveryBenchmarkTree(b, root, workload.files, workload.size, 'a')
			for _, syncFiles := range []bool{false, true} {
				b.Run(fmt.Sprintf("tree_hash/sync=%t", syncFiles), func(b *testing.B) {
					b.SetBytes(int64(workload.files * workload.size))
					b.ReportAllocs()

					for b.Loop() {
						hash, err := recoveryTreeHash(ctx, root, syncFiles)
						if err != nil || hash == "" {
							b.Fatalf("recoveryTreeHash(sync=%t) = %q, %v, want a fingerprint", syncFiles, hash, err)
						}
					}
				})
			}
			b.Run("cleanup_inventory", func(b *testing.B) {
				b.SetBytes(int64(workload.files * workload.size))
				b.ReportAllocs()

				for b.Loop() {
					entries, err := recoveryCleanupEntries(ctx, root)
					if err != nil || len(entries) < workload.files {
						b.Fatalf("recoveryCleanupEntries = %d entries, %v, want at least %d", len(entries), err, workload.files)
					}
				}
			})
		})
	}
}

func BenchmarkDependencyRecoveryValidation(b *testing.B) {
	ctx := context.Background()

	b.Run("no_journal", func(b *testing.B) {
		project := b.TempDir()
		writeRecoveryBenchmarkTree(b, filepath.Join(project, "dependencies", "library"), 2048, 512, 'a')

		b.ReportAllocs()

		for b.Loop() {
			status, err := InspectDependencyRecovery(ctx, project)
			if err != nil || status.Pending {
				b.Fatalf("InspectDependencyRecovery(no journal) = %+v, %v, want no pending recovery", status, err)
			}
		}
	})

	for _, workload := range recoveryBenchmarkWorkloads {
		b.Run(workload.name, func(b *testing.B) {
			for _, state := range []string{"prepared", "promoted", "committed", "committed_cleanup", "restored_cleanup"} {
				b.Run(state, func(b *testing.B) {
					journal := newRecoveryBenchmarkJournal(b, workload.files, workload.size, state)
					b.SetBytes(int64(2 * workload.files * workload.size))
					b.ReportAllocs()

					for b.Loop() {
						if err := journal.validateState(ctx); err != nil {
							b.Fatalf("validateState(%s) = %v, want valid recovery state", state, err)
						}
					}
				})
			}
		})
	}
}

func writeRecoveryBenchmarkTree(b *testing.B, root string, files, size int, fill byte) {
	b.Helper()
	data := bytes.Repeat([]byte{fill}, size)
	for i := 0; i < files; i++ {
		path := filepath.Join(root, fmt.Sprintf("include-%03d", i/32), fmt.Sprintf("file-%04d.inc", i))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

func newRecoveryBenchmarkJournal(b *testing.B, files, size int, state string) *dependencyJournal {
	b.Helper()

	ctx := context.Background()
	journal := &dependencyJournal{Version: 1, ID: "benchmark", Operation: "ensure", Phase: "active", project: b.TempDir()}
	stage := dependencyRecoveryStage{Path: "dependencies/.sampctl-stage-benchmark-0", Allowed: []string{"library", ".previous-0"}}
	change := dependencyRecoveryChange{Source: stage.Path + "/library", Target: "dependencies/library", Backup: stage.Path + "/.previous-0", Phase: "prepared"}
	writeRecoveryBenchmarkTree(b, journal.path(change.Source), files, size, 'n')
	writeRecoveryBenchmarkTree(b, journal.path(change.Target), files, size, 'o')

	var err error
	change.NewHash, err = recoveryTreeHash(ctx, journal.path(change.Source), false)
	if err != nil {
		b.Fatal(err)
	}

	change.OldHash, err = recoveryTreeHash(ctx, journal.path(change.Target), false)
	if err != nil {
		b.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(journal.path(stage.Path), ".sampctl-owner"), []byte(journal.ID), 0o600); err != nil {
		b.Fatal(err)
	}

	if state != "prepared" && state != "restored_cleanup" {
		if err := os.Rename(journal.path(change.Target), journal.path(change.Backup)); err != nil {
			b.Fatal(err)
		}
		if err := os.Rename(journal.path(change.Source), journal.path(change.Target)); err != nil {
			b.Fatal(err)
		}
		change.Phase = "promoted"
	}

	if state == "committed" || state == "committed_cleanup" {
		journal.Phase = "committed"
	} else if state == "restored_cleanup" {
		journal.Phase = "restored"
	}

	if state == "committed_cleanup" || state == "restored_cleanup" {
		stage.Cleanup = true
		stage.CleanupEntries, err = recoveryCleanupEntries(ctx, journal.path(stage.Path))
		if err != nil {
			b.Fatal(err)
		}
	}

	journal.Stages = []dependencyRecoveryStage{stage}
	journal.Changes = []dependencyRecoveryChange{change}
	return journal
}
