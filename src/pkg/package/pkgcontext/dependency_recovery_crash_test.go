package pkgcontext

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

const dependencyRecoveryCrashExit = 83

func TestDependencyRecoveryAfterProcessExit(t *testing.T) {
	for _, boundary := range []string{"before staging", "during staging", "after staging", "after backup", "after promotion", "after manifest", "after lockfile", "during rollback", "after rollback restore", "during committed cleanup"} {
		t.Run(boundary, func(t *testing.T) {
			project := t.TempDir()
			pkg := pawnpackage.Package{
				Format: "json", LocalPath: project, Parent: true, User: "fixture", Repo: "root",
				Dependencies: []versioning.DependencyString{"fixture/dependency:1.0.0"},
			}
			if err := pkg.WriteDefinition(); err != nil {
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
			manifestBefore := readCrashRecoveryFile(t, filepath.Join(project, "pawn.json"))
			lockBefore := readCrashRecoveryFile(t, filepath.Join(project, lockfile.Filename))

			command := exec.Command(os.Args[0], "-test.run=^TestDependencyRecoveryCrashHelper$")
			command.Dir = t.TempDir()
			command.Env = append(os.Environ(), "SAMPCTL_RECOVERY_CRASH_PROJECT="+project, "SAMPCTL_RECOVERY_CRASH_BOUNDARY="+boundary)
			output, err := command.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != dependencyRecoveryCrashExit {
				t.Fatalf("crash helper at %q: exit = %v, want %d; output:\n%s", boundary, err, dependencyRecoveryCrashExit, output)
			}
			if _, err := os.Lstat(filepath.Join(project, ".sampctl-dependencies.json")); err != nil {
				t.Fatalf("crash at %q left no recovery metadata: %v", boundary, err)
			}
			journalBefore := readCrashRecoveryFile(t, filepath.Join(project, DependencyRecoveryFilename))
			manifestAtCrash := readCrashRecoveryFile(t, filepath.Join(project, "pawn.json"))
			lockAtCrash := readCrashRecoveryFile(t, filepath.Join(project, lockfile.Filename))
			status, err := InspectDependencyRecovery(context.Background(), project)
			if err != nil {
				t.Fatalf("inspect crash at %q: %v", boundary, err)
			}
			if !status.Pending || !status.CanRecover || status.Active || status.Committed != (boundary == "during committed cleanup") {
				t.Errorf("inspection at %q = %+v, want recoverable inactive operation with matching commit state", boundary, status)
			}
			for path, want := range map[string][]byte{DependencyRecoveryFilename: journalBefore, "pawn.json": manifestAtCrash, lockfile.Filename: lockAtCrash} {
				if got := readCrashRecoveryFile(t, filepath.Join(project, path)); !bytes.Equal(got, want) {
					t.Errorf("read-only inspection changed %s at %q", path, boundary)
				}
			}
			if _, err := RecoverDependencyOperation(context.Background(), project); err != nil {
				t.Fatalf("recover crash at %q: %v", boundary, err)
			}
			wantDependency := "previous"
			if boundary == "during committed cleanup" {
				wantDependency = "replacement"
				manifest := readCrashRecoveryFile(t, filepath.Join(project, "pawn.json"))
				if bytes.Equal(manifest, manifestBefore) {
					t.Error("recovery reverted a committed manifest")
				}
				currentLock, err := lockfile.Load(project)
				if err != nil {
					t.Fatal(err)
				}
				if got := currentLock.Dependencies["github.com/fixture/dependency"].Commit; got != "2222222222222222222222222222222222222222" {
					t.Errorf("committed lock commit = %s, want replacement commit", got)
				}
			} else {
				if got := readCrashRecoveryFile(t, filepath.Join(project, "pawn.json")); !bytes.Equal(got, manifestBefore) {
					t.Errorf("manifest after recovery at %q = %s, want original %s", boundary, got, manifestBefore)
				}
				if got := readCrashRecoveryFile(t, filepath.Join(project, lockfile.Filename)); !bytes.Equal(got, lockBefore) {
					t.Errorf("lockfile after recovery at %q differs from its original snapshot", boundary)
				}
			}
			if got := string(readCrashRecoveryFile(t, filepath.Join(dependency, "keep.inc"))); got != wantDependency {
				t.Errorf("dependency after recovery at %q = %q, want %q", boundary, got, wantDependency)
			}
			if _, err := os.Lstat(filepath.Join(project, ".sampctl-dependencies.json")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("completed recovery at %q retained metadata: %v", boundary, err)
			}
			if _, err := RecoverDependencyOperation(context.Background(), project); err != nil {
				t.Errorf("repeated recovery at %q: %v", boundary, err)
			}
		})
	}
}

func TestDependencyRecoveryCrashHelper(t *testing.T) {
	project := os.Getenv("SAMPCTL_RECOVERY_CRASH_PROJECT")
	if project == "" {
		return
	}
	boundary := os.Getenv("SAMPCTL_RECOVERY_CRASH_BOUNDARY")
	pkg, err := pawnpackage.PackageFromDir(project)
	if err != nil {
		t.Fatal(err)
	}
	pkg.LocalPath = project
	pkg.Parent = true
	pkg.Vendor = filepath.Join(project, "dependencies")
	pcx := &PackageContext{Package: pkg}
	resolver, err := lockfile.NewResolver(project, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	pcx.SetLockfileResolver(resolver)
	state, err := pcx.beginDependencyOperation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if boundary == "before staging" {
		os.Exit(dependencyRecoveryCrashExit)
	}
	stage, err := pcx.dependencyInstalls.stage(pkg.Vendor, "dependency")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(stage, "dependency")
	if boundary == "during staging" {
		writeCrashRecoveryContent(t, source, "incomplete")
		os.Exit(dependencyRecoveryCrashExit)
	}
	writeCrashRecoveryContent(t, source, "replacement")
	if boundary == "after staging" {
		os.Exit(dependencyRecoveryCrashExit)
	}
	target := filepath.Join(pkg.Vendor, "dependency")
	pcx.dependencyInstalls.rename = func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		if boundary == "after backup" && from == target || boundary == "after promotion" && from == source {
			os.Exit(dependencyRecoveryCrashExit)
		}
		return nil
	}
	if err := pcx.dependencyInstalls.promote(source, target); err != nil {
		t.Fatal(err)
	}
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dependency:2.0.0"}
	if err := pcx.writeDependencyDefinition(); err != nil {
		t.Fatal(err)
	}
	if boundary == "after manifest" {
		os.Exit(dependencyRecoveryCrashExit)
	}
	entry := resolver.GetLockfile().Dependencies["github.com/fixture/dependency"]
	entry.Constraint = ":2.0.0"
	entry.Commit = "2222222222222222222222222222222222222222"
	resolver.GetLockfile().Dependencies["github.com/fixture/dependency"] = entry
	if err := pcx.saveDependencyLockfile(); err != nil {
		t.Fatal(err)
	}
	if boundary == "after lockfile" {
		os.Exit(dependencyRecoveryCrashExit)
	}
	if boundary == "during committed cleanup" {
		pcx.dependencyInstalls.removeAll = func(string) error { os.Exit(dependencyRecoveryCrashExit); return nil }
	}
	var operationErr error
	if boundary == "during rollback" || boundary == "after rollback restore" {
		backup := pcx.dependencyInstalls.installations[0].backup
		pcx.dependencyInstalls.rename = func(from, to string) error {
			if err := os.Rename(from, to); err != nil {
				return err
			}
			if boundary == "during rollback" && from == target || boundary == "after rollback restore" && from == backup {
				os.Exit(dependencyRecoveryCrashExit)
			}
			return nil
		}
		operationErr = errors.New("injected operation failure")
	}
	if err := pcx.finishDependencyOperation(state, operationErr); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("unknown crash boundary %q", boundary)
}

func writeCrashRecoveryContent(t *testing.T, directory, contents string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "keep.inc"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readCrashRecoveryFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
