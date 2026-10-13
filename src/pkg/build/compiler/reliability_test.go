package compiler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func compilerReliabilityFixture(t *testing.T) (CompilerFetchRequest, download.Compiler, string) {
	t.Helper()
	root := t.TempDir()
	request := CompilerFetchRequest{
		Meta:     versioning.DependencyMeta{User: "pawn-lang", Repo: "compiler", Tag: "v3.10.8"},
		Dir:      filepath.Join(root, "cache", "pawn", "pawn-lang", "compiler", "v3.10.8"),
		CacheDir: filepath.Join(root, "cache"), Platform: "linux",
	}
	pkg := seedCompilerCacheFixture(t, request.CacheDir, request.Meta, request.Platform)
	return request, pkg, cachedCompilerAssetPath(t, request.CacheDir, request.Meta, pkg)
}

func TestCompilerInstallPreservesSupportingFilesAndVerifiesContents(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	archive := filepath.Join(t.TempDir(), "compiler.tgz")
	pkg.Paths = map[string]string{"distribution/bin/pawncc": "pawncc", "distribution/lib/libpawnc.so": "libpawnc.so"}
	createCompilerArchive(t, archive, pkg.Method, map[string]string{
		"distribution/bin/pawncc": "#!/bin/sh\nexit 0\n", "distribution/lib/libpawnc.so": "library",
		"distribution/include/default.inc": "#define SUPPORT 1\n", "distribution/bin/pawndisasm": "disassembler",
	})
	require.NoError(t, installCompilerArchive(context.Background(), archive, request.Dir, request.Meta, request.Platform, pkg))
	for _, file := range []string{"pawncc", "libpawnc.so", "distribution/bin/pawncc", "distribution/lib/libpawnc.so", "distribution/include/default.inc", "distribution/bin/pawndisasm"} {
		require.FileExists(t, filepath.Join(request.Dir, file))
	}
	require.NoError(t, verifyCompilerInstallation(context.Background(), request.Dir, request.Meta, request.Platform, pkg))
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, "distribution/include/default.inc"), []byte("changed"), 0o600))
	require.Error(t, verifyCompilerInstallation(context.Background(), request.Dir, request.Meta, request.Platform, pkg))
}

func TestCompilerRepairUsesCachedArchiveAndInspectionIsReadOnly(t *testing.T) {
	request, _, archive := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, "libpawnc.so"), []byte("corrupt"), 0o600))
	before, err := compilerTreeHash(context.Background(), filepath.Dir(request.Dir))
	require.NoError(t, err)
	status, err := InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.True(t, status.CanRepair)
	after, hashErr := compilerTreeHash(context.Background(), filepath.Dir(request.Dir))
	require.NoError(t, hashErr)
	require.Equal(t, before, after)
	require.NoError(t, RepairManagedCompiler(context.Background(), request))
	status, err = InspectManagedCompiler(context.Background(), request)
	require.NoError(t, err)
	require.True(t, status.Valid)
	require.NoError(t, os.Remove(archive))
	require.NoError(t, os.Remove(filepath.Join(request.Dir, "pawncc")))
	status, err = InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.False(t, status.CanRepair)
	require.Error(t, RepairManagedCompiler(context.Background(), request))
}

func TestCompilerIncompleteLegacyCacheRepairAndUnknownFiles(t *testing.T) {
	request, _, _ := compilerReliabilityFixture(t)
	require.NoError(t, os.MkdirAll(request.Dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, "pawncc"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, "notes.txt"), []byte("mine"), 0o600))
	require.NoError(t, os.Remove(filepath.Join(request.Dir, "libpawnc.so")))
	status, err := InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.False(t, status.CanRepair)
	require.Error(t, RepairManagedCompiler(context.Background(), request))
	contents, err := os.ReadFile(filepath.Join(request.Dir, "notes.txt"))
	require.NoError(t, err)
	require.Equal(t, "mine", string(contents))
}

func TestCompilerArchiveFailuresPreservePreviousInstallation(t *testing.T) {
	for _, failure := range []string{"missing", "corrupt", "traversal"} {
		t.Run(failure, func(t *testing.T) {
			request, pkg, _ := compilerReliabilityFixture(t)
			_, err := GetCompilerPackage(context.Background(), request)
			require.NoError(t, err)
			before, err := compilerTreeHash(context.Background(), request.Dir)
			require.NoError(t, err)
			archive := filepath.Join(t.TempDir(), "broken.tgz")
			switch failure {
			case "missing":
				createCompilerArchive(t, archive, pkg.Method, map[string]string{"libpawnc.so": "lib"})
			case "corrupt":
				require.NoError(t, os.WriteFile(archive, []byte("not an archive"), 0o600))
			case "traversal":
				createCompilerArchive(t, archive, pkg.Method, map[string]string{"../escape": "no", "pawncc": "compiler", "libpawnc.so": "lib"})
			}
			require.Error(t, installCompilerArchive(context.Background(), archive, request.Dir, request.Meta, request.Platform, pkg))
			after, err := compilerTreeHash(context.Background(), request.Dir)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoFileExists(t, filepath.Join(filepath.Dir(request.Dir), "escape"))
			require.NoFileExists(t, compilerJournalPath(request.Dir))
			stages, err := filepath.Glob(filepath.Join(filepath.Dir(request.Dir), "."+filepath.Base(request.Dir)+".stage-*"))
			require.NoError(t, err)
			require.Empty(t, stages)
		})
	}
}

func TestCompilerFailedPromotionRestoresPreviousInstallation(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	archive := filepath.Join(t.TempDir(), "replacement.tgz")
	createCompilerArchive(t, archive, pkg.Method, map[string]string{"pawncc": "new compiler", "libpawnc.so": "new library"})
	before, err := compilerTreeHash(context.Background(), request.Dir)
	require.NoError(t, err)
	injected := errors.New("injected promotion failure")
	err = installCompilerArchiveWithHook(context.Background(), archive, request.Dir, request.Meta, request.Platform, pkg, func(point string) error {
		if point == "promoted" {
			return injected
		}
		return nil
	})
	require.ErrorIs(t, err, injected)
	var installErr *InstallationError
	require.ErrorAs(t, err, &installErr)
	require.True(t, installErr.Restored)
	require.False(t, installErr.RecoveryPending)
	after, err := compilerTreeHash(context.Background(), request.Dir)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestCompilerConcurrentInstallationAndCancellation(t *testing.T) {
	request, _, _ := compilerReliabilityFixture(t)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := GetCompilerPackage(context.Background(), request); failures <- err })
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	status, err := InspectManagedCompiler(context.Background(), request)
	require.NoError(t, err)
	require.True(t, status.Valid)
	unlock, err := lockCompilerInstallation(context.Background(), request.Dir)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetCompilerPackage(ctx, request)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, unlock())
}

func TestCompilerCrashRecovery(t *testing.T) {
	for _, point := range []string{"prepared", "backed-up", "promoted", "committed"} {
		t.Run(point, func(t *testing.T) {
			request, pkg, _ := compilerReliabilityFixture(t)
			_, err := GetCompilerPackage(context.Background(), request)
			require.NoError(t, err)
			archive := filepath.Join(t.TempDir(), "replacement.tgz")
			createCompilerArchive(t, archive, pkg.Method, map[string]string{"pawncc": "new compiler", "libpawnc.so": "new library"})
			crashCompilerInstaller(t, request, pkg, archive, point)
			status, err := InspectManagedCompiler(context.Background(), request)
			require.NoError(t, err)
			require.True(t, status.Pending)
			require.True(t, status.CanRepair)
			require.Equal(t, point == "committed", status.Committed)
			require.NoError(t, recoverCompilerInstallation(context.Background(), request.Dir))
			require.NoError(t, recoverCompilerInstallation(context.Background(), request.Dir))
			contents, err := os.ReadFile(filepath.Join(request.Dir, "pawncc"))
			require.NoError(t, err)
			want := "#!/bin/sh\nexit 0\n"
			if point == "committed" {
				want = "new compiler"
			}
			require.Equal(t, want, string(contents))
			require.NoError(t, verifyCompilerInstallation(context.Background(), request.Dir, request.Meta, request.Platform, pkg))
		})
	}
}

func TestCompilerCrashRecoveryRejectsModifiedBackup(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	archive := filepath.Join(t.TempDir(), "replacement.tgz")
	createCompilerArchive(t, archive, pkg.Method, map[string]string{"pawncc": "new compiler", "libpawnc.so": "new library"})
	crashCompilerInstaller(t, request, pkg, archive, "backed-up")
	journal, err := readCompilerJournal(request.Dir)
	require.NoError(t, err)
	backup := filepath.Join(filepath.Dir(request.Dir), journal.Backup)
	require.NoError(t, os.WriteFile(filepath.Join(backup, "libpawnc.so"), []byte("user change"), 0o600))
	before, err := compilerTreeHash(context.Background(), backup)
	require.NoError(t, err)
	status, err := InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.False(t, status.CanRepair)
	require.Error(t, recoverCompilerInstallation(context.Background(), request.Dir))
	after, err := compilerTreeHash(context.Background(), backup)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

type compilerCrashInput struct {
	Request CompilerFetchRequest
	Package download.Compiler
	Archive string
	Point   string
}

func crashCompilerInstaller(t *testing.T, request CompilerFetchRequest, pkg download.Compiler, archive, point string) {
	t.Helper()
	input := filepath.Join(t.TempDir(), "input.json")
	data, err := json.Marshal(compilerCrashInput{Request: request, Package: pkg, Archive: archive, Point: point})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(input, data, 0o600))
	cmd := exec.Command(os.Args[0], "-test.run=^TestCompilerInstallerCrashHelper$")
	cmd.Env = append(os.Environ(), "SAMPCTL_COMPILER_CRASH_INPUT="+input)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, string(output))
	require.Equal(t, 86, exitErr.ExitCode(), string(output))
}

func TestCompilerInstallerCrashHelper(t *testing.T) {
	input := os.Getenv("SAMPCTL_COMPILER_CRASH_INPUT")
	if input == "" {
		return
	}
	data, err := os.ReadFile(input)
	require.NoError(t, err)
	var request compilerCrashInput
	require.NoError(t, json.Unmarshal(data, &request))
	unlock, err := lockCompilerInstallation(context.Background(), request.Request.Dir)
	require.NoError(t, err)
	defer unlock() //nolint:errcheck
	require.NoError(t, installCompilerArchiveWithHook(context.Background(), request.Archive, request.Request.Dir, request.Request.Meta, request.Request.Platform, request.Package, func(point string) error {
		if request.Point == "rollback-cleanup-entry" && point == "promoted" {
			return errors.New("injected rollback")
		}
		if request.Point == "rollback-cleanup-entry" && point == "cleanup-entry" {
			os.Exit(86)
		}
		if point == request.Point {
			os.Exit(86)
		}
		return nil
	}))
	t.Fatal("installer did not reach crash point")
}

func TestCompilerExecutionErrorsAndPawnDiagnostics(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ mode, kind string }{{"source", ""}, {"crash", ExecutionCrash}, {"output", ExecutionOutput}}
	for _, test := range cases {
		t.Run(test.mode, func(t *testing.T) {
			cmd := compilerProcessCommand(test.mode, "")
			problems, _, err := CompileWithCommandContext(context.Background(), cmd, dir, dir, true)
			if test.kind == "" {
				require.NoError(t, err)
				require.Len(t, problems, 1)
				require.Equal(t, "001", problems[0].Code)
				require.Equal(t, 12, problems[0].Line)
				require.Equal(t, "script.pwn", problems[0].File)
				return
			}
			var execErr *ExecutionError
			require.ErrorAs(t, err, &execErr)
			require.Equal(t, test.kind, execErr.Kind)
			require.NotEmpty(t, execErr.Output)
		})
	}
	cmd := exec.Command(filepath.Join(dir, "missing"))
	_, _, err := CompileWithCommandContext(context.Background(), cmd, dir, dir, true)
	var executionErr *ExecutionError
	require.ErrorAs(t, err, &executionErr)
	require.Equal(t, ExecutionMissing, executionErr.Kind)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = CompileWithCommandContext(ctx, compilerProcessCommand("source", ""), dir, dir, true)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorAs(t, err, &executionErr)
	require.Equal(t, ExecutionCancelled, executionErr.Kind)
}

func compilerProcessCommand(mode, ready string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCompilerProcessHelper$")
	cmd.Env = append(os.Environ(), "SAMPCTL_COMPILER_PROCESS_HELPER="+mode, "SAMPCTL_COMPILER_PROCESS_READY="+ready)
	return cmd
}

func TestCompilerProcessHelper(t *testing.T) {
	switch os.Getenv("SAMPCTL_COMPILER_PROCESS_HELPER") {
	case "source":
		println("script.pwn(12) : error 001: invalid declaration")
		os.Exit(1)
	case "crash":
		println("compiler stopped unexpectedly")
		os.Exit(2)
	case "output":
		println("script.pwn(12) : fatal error 101: cannot write to file")
		os.Exit(1)
	case "wait":
		ready := os.Getenv("SAMPCTL_COMPILER_PROCESS_READY")
		require.NoError(t, os.WriteFile(ready, []byte("ready"), 0o600))
		for {
			if _, err := os.Stat(ready + ".release"); err == nil {
				os.Exit(0)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func TestCompilerExternalPathAndUnsafeManagedFilesArePreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic link creation requires Windows privileges")
	}
	request, _, _ := compilerReliabilityFixture(t)
	external := filepath.Join(t.TempDir(), "external")
	require.NoError(t, os.WriteFile(external, []byte("external compiler"), 0o600))
	require.NoError(t, os.MkdirAll(request.Dir, 0o700))
	require.NoError(t, os.Symlink(external, filepath.Join(request.Dir, "pawncc")))
	_, err := GetCompilerPackage(context.Background(), request)
	require.Error(t, err)
	contents, err := os.ReadFile(external)
	require.NoError(t, err)
	require.Equal(t, "external compiler", string(contents))
	info, err := os.Lstat(filepath.Join(request.Dir, "pawncc"))
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink)
	_, err = compilerFromCustomPath(filepath.Dir(external))
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "repair"))
}

func TestCompilerExecutionPreventsConcurrentRepair(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	binary := filepath.Join(request.Dir, "pawncc")
	require.NoError(t, os.Remove(binary))
	require.NoError(t, copyCompilerFile(context.Background(), os.Args[0], binary))
	require.NoError(t, os.Chmod(binary, 0o700))
	completion, err := readCompilerCompletion(request.Dir)
	require.NoError(t, err)
	completion.Files, err = compilerInventory(context.Background(), request.Dir, false)
	require.NoError(t, err)
	data, err := json.Marshal(completion)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, compilerCompletionFilename), data, 0o600))
	require.NoError(t, verifyCompilerInstallation(context.Background(), request.Dir, request.Meta, request.Platform, pkg))
	ready := filepath.Join(t.TempDir(), "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestCompilerProcessHelper$")
	cmd.Env = append(os.Environ(), "SAMPCTL_COMPILER_PROCESS_HELPER=wait", "SAMPCTL_COMPILER_PROCESS_READY="+ready)
	finished := make(chan error, 1)
	go func() {
		_, _, err := CompileWithCommandContext(ctx, cmd, request.Dir, request.Dir, true)
		finished <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("compiler helper did not start")
		}
		time.Sleep(time.Millisecond)
	}
	status, err := InspectManagedCompiler(context.Background(), request)
	require.NoError(t, err)
	require.True(t, status.Valid, "inspection can share a compiler execution lease")
	repairCtx, repairCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err = RepairManagedCompiler(repairCtx, request)
	repairCancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, os.WriteFile(ready+".release", []byte("release"), 0o600))
	require.NoError(t, <-finished)
	require.NoError(t, RepairManagedCompiler(context.Background(), request))
}

func TestCompilerCancellationRestoresPreviousInstallation(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	before, err := compilerTreeHash(context.Background(), request.Dir)
	require.NoError(t, err)
	archive := filepath.Join(t.TempDir(), "replacement.tgz")
	createCompilerArchive(t, archive, pkg.Method, map[string]string{"pawncc": "new compiler", "libpawnc.so": "new library"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = installCompilerArchiveWithHook(ctx, archive, request.Dir, request.Meta, request.Platform, pkg, func(point string) error {
		if point == "backed-up" {
			cancel()
		}
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	var installationErr *InstallationError
	require.ErrorAs(t, err, &installationErr)
	require.True(t, installationErr.Restored)
	after, err := compilerTreeHash(context.Background(), request.Dir)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestCompilerCrashDuringCleanupCanResume(t *testing.T) {
	for _, point := range []string{"cleanup-entry", "rollback-cleanup-entry"} {
		t.Run(point, func(t *testing.T) {
			request, pkg, _ := compilerReliabilityFixture(t)
			_, err := GetCompilerPackage(context.Background(), request)
			require.NoError(t, err)
			archive := filepath.Join(t.TempDir(), "replacement.tgz")
			createCompilerArchive(t, archive, pkg.Method, map[string]string{"pawncc": "new compiler", "libpawnc.so": "new library"})
			crashCompilerInstaller(t, request, pkg, archive, point)
			status, err := InspectManagedCompiler(context.Background(), request)
			require.NoError(t, err)
			require.True(t, status.Pending)
			require.True(t, status.CanRepair)
			require.NoError(t, recoverCompilerInstallation(context.Background(), request.Dir))
			require.NoError(t, recoverCompilerInstallation(context.Background(), request.Dir))
			contents, err := os.ReadFile(filepath.Join(request.Dir, "pawncc"))
			require.NoError(t, err)
			want := "new compiler"
			if point == "rollback-cleanup-entry" {
				want = "#!/bin/sh\nexit 0\n"
			}
			require.Equal(t, want, string(contents))
			require.NoFileExists(t, compilerJournalPath(request.Dir))
		})
	}
}

func TestCompilerRepairPreservesUnknownEmptyDirectories(t *testing.T) {
	request, _, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	unknown := filepath.Join(request.Dir, "manual-notes")
	require.NoError(t, os.Mkdir(unknown, 0o700))
	require.NoError(t, os.Remove(filepath.Join(request.Dir, "libpawnc.so")))
	require.Error(t, RepairManagedCompiler(context.Background(), request))
	require.DirExists(t, unknown)
}

func TestCompilerCancellationStopsWaitingForDescendantOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 2 & echo compiler-started; wait")
	cmd.WaitDelay = 100 * time.Millisecond
	start := time.Now()
	_, _, err := CompileWithCommandContext(ctx, cmd, t.TempDir(), "", true)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

func TestCompilerContextCancelsUnboundCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cmd := exec.Command("/bin/sh", "-c", "sleep 2")
	cmd.WaitDelay = 100 * time.Millisecond
	start := time.Now()
	_, _, err := CompileWithCommandContext(ctx, cmd, t.TempDir(), "", true)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

func TestCompilerPermissionRepairPreservesCompletionMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX permission semantics")
	}
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(request.Dir, pkg.Binary), 0o600))
	require.NoError(t, RepairManagedCompilerPermissions(context.Background(), request.Dir, pkg.Binary))
	require.NoError(t, verifyCompilerInstallation(context.Background(), request.Dir, request.Meta, request.Platform, pkg))
}

func TestCompilerNestedExecutableUsesInstallationLock(t *testing.T) {
	request, _, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(request.Dir, "bin"), 0o700))
	nested := filepath.Join(request.Dir, "bin", "pawncc")
	require.NoError(t, os.Rename(filepath.Join(request.Dir, "pawncc"), nested))
	completion, err := readCompilerCompletion(request.Dir)
	require.NoError(t, err)
	completion.Package.Binary = "bin/pawncc"
	completion.Package.Paths = map[string]string{"bin/pawncc": "bin/pawncc", "libpawnc.so": "libpawnc.so"}
	completion.Files, err = compilerInventory(context.Background(), request.Dir, false)
	require.NoError(t, err)
	data, err := json.Marshal(completion)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, compilerCompletionFilename), data, 0o600))
	unlock, err := lockCompilerExecution(context.Background(), exec.Command(nested))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = RepairManagedCompiler(ctx, request)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, unlock())
}

func TestCompilerCorruptCachedArchiveIsDownloadedAgain(t *testing.T) {
	request, _, archive := compilerReliabilityFixture(t)
	valid, err := os.ReadFile(archive)
	require.NoError(t, err)
	request.GitHub = newCompilerReleaseClient(t, request.Meta, offlineCompilerArchiveName(request.Platform, request.Meta.Tag), valid)
	require.NoError(t, os.WriteFile(archive, []byte("corrupt archive"), 0o600))
	_, err = GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	status, err := InspectManagedCompiler(context.Background(), request)
	require.NoError(t, err)
	require.True(t, status.Valid)
}

func TestCompilerPartialCleanupRejectsChangedRemainingFiles(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	archive := filepath.Join(t.TempDir(), "replacement.tgz")
	createCompilerArchive(t, archive, pkg.Method, map[string]string{"pawncc": "new compiler", "libpawnc.so": "new library"})
	crashCompilerInstaller(t, request, pkg, archive, "cleanup-entry")
	journal, err := readCompilerJournal(request.Dir)
	require.NoError(t, err)
	backup := filepath.Join(filepath.Dir(request.Dir), journal.Backup)
	require.NoError(t, os.WriteFile(filepath.Join(backup, "libpawnc.so"), []byte("user change"), 0o600))
	before, err := compilerTreeHash(context.Background(), backup)
	require.NoError(t, err)
	status, err := InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.False(t, status.CanRepair)
	require.Error(t, recoverCompilerInstallation(context.Background(), request.Dir))
	after, err := compilerTreeHash(context.Background(), backup)
	require.NoError(t, err)
	require.Equal(t, before, after)
	contents, err := os.ReadFile(filepath.Join(request.Dir, "pawncc"))
	require.NoError(t, err)
	require.Equal(t, "new compiler", string(contents))
}

func TestCompilerReadLeasesAllowConcurrentBuildsAndBlockInstallation(t *testing.T) {
	request, pkg, _ := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	cmd := exec.Command(filepath.Join(request.Dir, pkg.Binary))
	first, err := lockCompilerExecution(context.Background(), cmd)
	require.NoError(t, err)
	defer first() //nolint:errcheck
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	second, err := lockCompilerExecution(ctx, cmd)
	require.NoError(t, err, "another build must be able to share a verified compiler")
	_, hit, err := FromCacheContext(ctx, request)
	require.NoError(t, err, "a complete cache hit must not wait for other builds")
	require.True(t, hit)
	_, err = GetCompilerPackage(ctx, request)
	require.NoError(t, err)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer waitCancel()
	_, err = lockCompilerInstallation(waitCtx, request.Dir)
	require.ErrorIs(t, err, context.DeadlineExceeded, "installation must wait for every execution lease")
	require.NoError(t, second())
	waitCtx2, waitCancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer waitCancel2()
	_, err = lockCompilerInstallation(waitCtx2, request.Dir)
	require.ErrorIs(t, err, context.DeadlineExceeded, "one remaining execution lease must still block installation")
	require.NoError(t, first())
	install, err := lockCompilerInstallation(ctx, request.Dir)
	require.NoError(t, err)
	require.NoError(t, install())
}

func TestCompilerInspectionDistinguishesPermissionOnlyRepair(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX permission semantics")
	}
	request, pkg, archive := compilerReliabilityFixture(t)
	_, err := GetCompilerPackage(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, os.Remove(archive))
	require.NoError(t, os.Chmod(filepath.Join(request.Dir, pkg.Binary), 0o600))
	status, err := InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.True(t, status.HasCompletion)
	require.True(t, status.PermissionsOnly)
	require.False(t, status.CanRepair, "full reinstallation is unavailable without the archive")
	require.NoError(t, os.WriteFile(filepath.Join(request.Dir, "libpawnc.so"), []byte("edited library"), 0o600))
	status, err = InspectManagedCompiler(context.Background(), request)
	require.Error(t, err)
	require.True(t, status.HasCompletion)
	require.False(t, status.PermissionsOnly)
	require.False(t, status.CanRepair)
	require.Error(t, RepairManagedCompilerPermissions(context.Background(), request.Dir, pkg.Binary))
}

func TestCompilerMissingInterpreterIsStartFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX interpreter semantics")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "pawncc")
	require.NoError(t, os.WriteFile(binary, []byte("#!/sampctl/nonexistent/interpreter\n"), 0o700))
	_, _, err := CompileWithCommandContext(context.Background(), exec.Command(binary), dir, dir, true)
	var executionErr *ExecutionError
	require.ErrorAs(t, err, &executionErr)
	require.Equal(t, ExecutionStart, executionErr.Kind)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCompilerDescendantOutputTimeoutIsOutputFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 0.2 & echo compiler-finished")
	cmd.WaitDelay = 20 * time.Millisecond
	_, _, err := CompileWithCommandContext(context.Background(), cmd, t.TempDir(), "", true)
	var executionErr *ExecutionError
	require.ErrorAs(t, err, &executionErr)
	require.Equal(t, ExecutionOutput, executionErr.Kind)
	require.ErrorIs(t, err, exec.ErrWaitDelay)
}
