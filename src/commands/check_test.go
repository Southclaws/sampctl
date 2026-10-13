package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestCheckProject(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, contents string
		status         checkStatus
	}{
		{name: "valid", contents: `{}`, status: checkPassed},
		{name: "missing", status: checkSkipped},
		{name: "malformed", contents: "{\n invalid", status: checkFailed},
		{name: "invalid", contents: `{"entry":"main.pwn","output":"main.pwn"}`, status: checkFailed},
		{name: "null build", contents: `{"builds":[null]}`, status: checkFailed},
		{name: "null runtime", contents: `{"runtimes":[null]}`, status: checkFailed},
		{name: "null definition", contents: `null`, status: checkFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.contents != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(tc.contents), 0o600))
			}
			pkg, result := checkProject(dir)
			assert.Equal(t, tc.status, result.Status)
			if tc.status == checkPassed {
				assert.Equal(t, dir, pkg.LocalPath)
			}
			if tc.name == "malformed" {
				assert.Contains(t, result.Message, "line 2")
				assert.NotContains(t, result.Message, "invalid character")
			}
		})
	}
}

func TestCheckProjectDiscoversParentAndRejectsAmbiguousDefinitions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.yaml"), []byte("entry: main.pwn\n"), 0o600))
	child := filepath.Join(dir, "src", "nested")
	require.NoError(t, os.MkdirAll(child, 0o700))
	pkg, result := checkProject(child)
	assert.Equal(t, checkPassed, result.Status)
	assert.Equal(t, dir, pkg.LocalPath)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(`{}`), 0o600))
	_, result = checkProject(child)
	assert.Equal(t, checkFailed, result.Status)
	assert.Contains(t, result.Message, "both pawn.json and pawn.yaml")
}

func TestRepairDiagnostic(t *testing.T) {
	t.Parallel()
	repairErr := errors.New("repair failed")
	for _, tc := range []struct {
		name      string
		repairErr error
		verified  bool
		cancel    bool
	}{
		{name: "successful", verified: true},
		{name: "failed", repairErr: repairErr},
		{name: "failed cleanup", repairErr: repairErr, verified: true},
		{name: "verification failure"},
		{name: "cancellation", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := diagnostic{Name: "fixture", Status: checkFailed, CanFix: true, Message: "broken resource"}
			got, err := repairDiagnostic(ctx, d, func(ctx context.Context) error {
				if tc.cancel {
					cancel()
					return ctx.Err()
				}
				return tc.repairErr
			}, func() diagnostic {
				if tc.verified {
					return diagnostic{Name: "fixture", Status: checkPassed, Message: "resource available"}
				}
				return d
			})
			assert.True(t, got.RepairAttempted)
			assert.Equal(t, tc.verified, got.RepairVerified)
			if tc.cancel {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
			if tc.verified && tc.repairErr == nil {
				assert.Equal(t, checkRepaired, got.Status)
			} else {
				assert.Equal(t, checkFailed, got.Status)
			}
			if tc.repairErr != nil {
				assert.ErrorIs(t, got.Err, repairErr)
			}
		})
	}
}

func TestWriteCheckResults(t *testing.T) {
	t.Parallel()
	results := []diagnostic{
		{Name: "project", Status: checkPassed, Message: "Project configuration is valid"},
		{Name: "compiler", Status: checkFailed, Message: "Compiler is missing", Action: "Check the compiler path.", Path: "/fixture/compiler", Err: errors.New("underlying fixture error"), CanFix: true},
		{Name: "git", Status: checkWarning, Message: "Git is unavailable"},
		{Name: "lockfile", Status: checkSkipped, Message: "Lockfile check skipped"},
		{Name: "cache", Status: checkRepaired, Message: "Cache metadata repaired", RepairAttempted: true, RepairVerified: true},
	}
	var normal, verbose bytes.Buffer
	code, err := writeCheckResults(&normal, results, checkOutputOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, code)
	assert.Contains(t, normal.String(), "  ✗ Compiler is missing\n    → Check the compiler path.\n")
	assert.Contains(t, normal.String(), "  ! Warning: Git is unavailable")
	assert.Contains(t, normal.String(), "  – Skipped: Lockfile check skipped")
	assert.Contains(t, normal.String(), "  ✓ Repaired: Cache metadata repaired")
	assert.Contains(t, normal.String(), "1 issue, 1 warning, 1 repaired, 1 passed, 1 skipped.")
	assert.NotContains(t, normal.String(), "\x1b[", "redirected output must not contain colour codes")
	assert.Contains(t, normal.String(), "sampctl check --fix")
	assert.NotContains(t, normal.String(), "underlying fixture error")
	assert.NotContains(t, normal.String(), "/fixture/compiler")
	_, err = writeCheckResults(&verbose, results, checkOutputOptions{Verbose: true})
	require.NoError(t, err)
	assert.Contains(t, verbose.String(), "    Details: underlying fixture error")
	assert.Contains(t, verbose.String(), "    Path: /fixture/compiler")
	assert.Equal(t, checkPassed, results[0].Status, "rendering must not reorder the caller's results")
	assert.Less(t, bytes.Index(normal.Bytes(), []byte("Compiler is missing")), bytes.Index(normal.Bytes(), []byte("Git is unavailable")))
	assert.Less(t, bytes.Index(normal.Bytes(), []byte("Git is unavailable")), bytes.Index(normal.Bytes(), []byte("Project configuration is valid")))
}

func TestWriteCheckResultsSummary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		results []diagnostic
		code    int
		summary string
	}{
		{name: "passed", results: []diagnostic{{Status: checkPassed}}, summary: "1 passed."},
		{name: "warnings", results: []diagnostic{{Status: checkWarning}, {Status: checkWarning}}, summary: "2 warnings."},
		{name: "failures", results: []diagnostic{{Status: checkFailed}, {Status: checkFailed}}, code: 1, summary: "2 issues."},
		{name: "repaired", results: []diagnostic{{Status: checkRepaired}}, summary: "1 repaired."},
		{name: "skipped", results: []diagnostic{{Status: checkSkipped}}, summary: "1 skipped."},
		{name: "empty", summary: "No checks apply."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code, err := writeCheckResults(&out, tc.results, checkOutputOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.code, code)
			assert.Contains(t, out.String(), "\n"+tc.summary+"\n")
			assert.NotContains(t, out.String(), "All applicable checks passed")
		})
	}
}

func TestRunChecksAggregatesAndSkipsDependentChecks(t *testing.T) {
	t.Parallel()
	dir, cacheDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pawn.json"), []byte(`{"invalid"`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "config.json"), []byte(`{"invalid"`), 0o600))
	results, err := runChecks(context.Background(), checkOptions{Dir: dir, CacheDir: cacheDir, Platform: "linux"})
	require.NoError(t, err)
	states := make(map[string]checkStatus)
	for _, result := range results {
		states[result.Name] = result.Status
	}
	assert.Equal(t, checkFailed, states["project"])
	assert.Equal(t, checkFailed, states["configuration"])
	assert.Equal(t, checkSkipped, states["dependencies"])
	assert.Equal(t, checkSkipped, states["lockfile"])
	assert.Equal(t, checkSkipped, states["compiler"])
	assert.Contains(t, states, "cache")
	assert.Contains(t, states, "git")
}

func TestRunChecksOutsideProjectDoesNotCreateCache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "missing", "sampctl")
	results, err := runChecks(context.Background(), checkOptions{Dir: dir, CacheDir: cacheDir, Platform: "linux", Fix: true})
	require.NoError(t, err)
	var out bytes.Buffer
	code, err := writeCheckResults(&out, results, checkOutputOptions{})
	require.NoError(t, err)
	assert.Zero(t, code, out.String())
	_, err = os.Stat(cacheDir)
	assert.True(t, os.IsNotExist(err))
}

func TestRunChecksPreservesProjectAndExternalFiles(t *testing.T) {
	t.Parallel()
	dir, cacheDir := t.TempDir(), t.TempDir()
	compilerDir := filepath.Join(dir, "custom")
	require.NoError(t, os.Mkdir(compilerDir, 0o700))
	binary := "pawncc"
	if runtime.GOOS == "windows" {
		binary = "pawncc.exe"
	}
	files := map[string]string{
		filepath.Join(dir, "pawn.json"):                     `{"build":{"compiler":{"path":"custom"}}}`,
		filepath.Join(dir, "pawn.lock"):                     `{"version":1,"dependencies":{}}`,
		filepath.Join(compilerDir, binary):                  "This compiler must never execute during diagnostics.",
		filepath.Join(cacheDir, "config.json"):              `{"default_user":"fixture"}`,
		filepath.Join(cacheDir, "packages.json"):            "[",
		filepath.Join(cacheDir, ".packages.json.abandoned"): "user file",
	}
	before := make(map[string]os.FileInfo)
	for path, contents := range files {
		require.NoError(t, os.WriteFile(path, []byte(contents), fs.PermFileExec))
		info, err := os.Stat(path)
		require.NoError(t, err)
		before[path] = info
	}
	child := filepath.Join(dir, "nested")
	require.NoError(t, os.Mkdir(child, 0o700))
	opts := checkOptions{Dir: child, CacheDir: cacheDir, Platform: runtime.GOOS}
	results, err := runChecks(context.Background(), opts)
	require.NoError(t, err)
	assert.Contains(t, results, diagnostic{Name: "compiler", Status: checkPassed, Message: "Compiler is available", Path: filepath.Join(compilerDir, binary)})
	assertCheckFixtureUnchanged(t, files, before, "")
	opts.Fix = true
	for range 2 {
		_, err = runChecks(context.Background(), opts)
		require.NoError(t, err)
		assert.NoFileExists(t, filepath.Join(cacheDir, "packages.json"))
		assertCheckFixtureUnchanged(t, files, before, filepath.Join(cacheDir, "packages.json"))
	}
}

func assertCheckFixtureUnchanged(t *testing.T, files map[string]string, before map[string]os.FileInfo, removed string) {
	t.Helper()
	for path, contents := range files {
		if path == removed {
			continue
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, contents, string(data), path)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, before[path].Mode(), info.Mode(), path)
		assert.Equal(t, before[path].ModTime(), info.ModTime(), path)
	}
}

func TestCheckOutputWriteFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("writer failed")
	code, err := writeCheckResults(checkErrorWriter{err: cause}, nil, checkOutputOptions{})
	assert.Equal(t, 2, code)
	assert.ErrorIs(t, err, cause)
}

type checkErrorWriter struct{ err error }

func (w checkErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCheckDependenciesNeedGit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		deps []versioning.DependencyString
		need bool
	}{
		{name: "no dependencies"},
		{name: "local includes", deps: []versioning.DependencyString{"includes://local/include"}},
		{name: "local plugin", deps: []versioning.DependencyString{"plugin://local/plugins/plugin.so"}},
		{name: "remote", deps: []versioning.DependencyString{"owner/repo"}, need: true},
		{name: "remote include", deps: []versioning.DependencyString{"includes://owner/repo"}, need: true},
		{name: "invalid", deps: []versioning.DependencyString{"invalid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.need, checkDependenciesNeedGit(pawnpackage.Package{Dependencies: tc.deps}, nil))
		})
	}
}
