package pkgcontext

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func symlinkDependencyProject(t *testing.T, project string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(project, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	return link
}

func TestDependencyOperationRejectsSymlinkProjectBeforeMutatingFiles(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	project := pcx.Package.LocalPath
	deps := []versioning.DependencyString{"fixture/dependency:1.0.0"}
	pcx.Package.Dependencies = slices.Clone(deps)
	if err := pcx.Package.WriteDefinition(); err != nil {
		t.Fatal(err)
	}
	locked := lockfile.New("test")
	locked.Dependencies["github.com/fixture/dependency"] = lockfile.LockedDependency{
		User: "fixture", Repo: "dependency", Constraint: ":1.0.0", Commit: "1111111111111111111111111111111111111111",
	}
	if err := lockfile.Save(project, locked); err != nil {
		t.Fatal(err)
	}
	dependency := filepath.Join(project, "dependencies", "dependency")
	writeCrashRecoveryContent(t, dependency, "previous")
	before := make(map[string][]byte)
	for _, path := range []string{"pawn.json", lockfile.Filename, "dependencies/dependency/keep.inc"} {
		before[path] = readCrashRecoveryFile(t, filepath.Join(project, filepath.FromSlash(path)))
	}
	pcx.Package.LocalPath = symlinkDependencyProject(t, project)
	pcx.Package.Vendor = filepath.Join(pcx.Package.LocalPath, "dependencies")
	err := pcx.Uninstall([]versioning.DependencyString{"fixture/dependency"}, false)
	var operation *OperationError
	if !errors.As(err, &operation) || operation.Operation != "project" {
		t.Fatalf("Uninstall(symlink project) = %v, want project error", err)
	}
	if !slices.Equal(pcx.Package.Dependencies, deps) {
		t.Errorf("dependencies after rejected operation = %v, want %v", pcx.Package.Dependencies, deps)
	}
	for path, want := range before {
		if got := readCrashRecoveryFile(t, filepath.Join(project, filepath.FromSlash(path))); !bytes.Equal(got, want) {
			t.Errorf("rejected operation changed %s", path)
		}
	}
	for _, path := range []string{DependencyRecoveryFilename, ".sampctl-dependencies.lock"} {
		if _, err := os.Lstat(filepath.Join(project, path)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("rejected operation created %s: %v", path, err)
		}
	}
}

func TestDependencyRecoveryRejectsSymlinkProjectWithoutMutatingEvidence(t *testing.T) {
	fixture := newDependencyRecoveryFixture(t)
	project := fixture.pcx.Package.LocalPath
	link := symlinkDependencyProject(t, project)
	backup := fixture.state.journal.path(fixture.state.journal.Changes[0].Backup)
	before := make(map[string][]byte)
	for _, path := range []string{
		filepath.Join(project, "pawn.json"),
		filepath.Join(project, DependencyRecoveryFilename),
		filepath.Join(fixture.target, "keep.inc"),
		filepath.Join(backup, "keep.inc"),
		filepath.Join(fixture.stage, ".sampctl-owner"),
	} {
		before[path] = readCrashRecoveryFile(t, path)
	}
	releaseDependencyRecoveryFixture(t, fixture)
	_, err := RecoverDependencyOperation(context.Background(), link)
	var operation *OperationError
	if !errors.As(err, &operation) || operation.Operation != "project" {
		t.Fatalf("RecoverDependencyOperation(symlink project) = %v, want project error", err)
	}
	for path, want := range before {
		if got := readCrashRecoveryFile(t, path); !bytes.Equal(got, want) {
			t.Errorf("rejected recovery changed %s", path)
		}
	}
	if _, err := RecoverDependencyOperation(context.Background(), project); err != nil {
		t.Fatalf("recover using original project directory: %v", err)
	}
}
