package pkgcontext

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestDependencyRecoveryPreservesProjectWhenSnapshotIsCorrupt(t *testing.T) {
	for _, field := range []string{"data", "mode"} {
		t.Run(field, func(t *testing.T) {
			fixture := newDependencyRecoveryFixture(t)
			fixture.pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dependency:2.0.0"}
			if err := fixture.pcx.writeDependencyDefinition(); err != nil {
				t.Fatal(err)
			}

			manifest := filepath.Join(fixture.pcx.Package.LocalPath, "pawn.json")
			before := readCrashRecoveryFile(t, manifest)

			snapshot := &fixture.state.journal.Files[0]
			if field == "data" {
				snapshot.Data = []byte(`{"user":"corrupt snapshot"}`)
			} else {
				snapshot.Mode ^= 0o077
			}
			if err := fixture.state.journal.save(); err != nil {
				t.Fatal(err)
			}

			if err := fixture.state.lock.Unlock(); err != nil {
				t.Fatal(err)
			}

			if _, err := RecoverDependencyOperation(context.Background(), fixture.pcx.Package.LocalPath); err == nil {
				t.Error("corrupt snapshot recovery succeeded")
			}

			if got := readCrashRecoveryFile(t, manifest); !bytes.Equal(got, before) {
				t.Error("corrupt snapshot changed the project manifest")
			}
			assertRecoveryEvidence(t, fixture, "replacement")
		})
	}
}
