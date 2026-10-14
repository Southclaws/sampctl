package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestCheckDependencies(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, pawnpackage.Package)
		deps  []versioning.DependencyString
		want  checkStatus
	}{
		{name: "no dependencies", want: checkPassed},
		{name: "missing dependency", deps: []versioning.DependencyString{"owner/missing"}, want: checkFailed},
		{name: "invalid declaration", deps: []versioning.DependencyString{"invalid"}, want: checkFailed},
		{name: "incomplete installation", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			require.NoError(t, os.MkdirAll(filepath.Join(pkg.Vendor, "repo"), 0o755))
		}, want: checkFailed},
		{name: "installed dependency without package metadata", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), "")
		}, want: checkPassed},
		{name: "missing transitive dependency", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), `{"dependencies":["other/transitive"]}`)
		}, want: checkFailed},
		{name: "transitive development dependencies excluded", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), `{"dev_dependencies":["other/development"]}`)
		}, want: checkPassed},
		{name: "missing include subdirectory", deps: []versioning.DependencyString{"owner/repo/includes"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), "")
		}, want: checkFailed},
		{name: "distinct include paths checked independently", deps: []versioning.DependencyString{"owner/repo/first", "owner/repo/second"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			path := filepath.Join(pkg.Vendor, "repo")
			newCheckDependencyRepo(t, path, "")
			require.NoError(t, os.Mkdir(filepath.Join(path, "first"), 0o755))
		}, want: checkFailed},
		{name: "missing package include path", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), `{"include_path":"includes"}`)
		}, want: checkFailed},
		{name: "malformed installed package", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), "{")
		}, want: checkFailed},
		{name: "broken package definition symlink", deps: []versioning.DependencyString{"owner/repo"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			path := filepath.Join(pkg.Vendor, "repo")
			newCheckDependencyRepo(t, path, "")
			if err := os.Symlink("missing.json", filepath.Join(path, "pawn.json")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}, want: checkFailed},
		{name: "missing local include", deps: []versioning.DependencyString{"includes://local/../external"}, want: checkFailed},
		{name: "local include may be outside project", deps: []versioning.DependencyString{"includes://local/../external"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			require.NoError(t, os.MkdirAll(filepath.Join(pkg.LocalPath, "..", "external"), 0o755))
		}, want: checkPassed},
		{name: "local plugin package include path is checked", deps: []versioning.DependencyString{"plugin://local/plugins/local"}, setup: func(t *testing.T, pkg pawnpackage.Package) {
			path := filepath.Join(pkg.LocalPath, "plugins", "local")
			require.NoError(t, os.MkdirAll(path, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(path, "pawn.json"), []byte(`{"include_path":"missing"}`), 0o644))
		}, want: checkFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := newCheckDependenciesPackage(t)
			pkg.Dependencies = tt.deps
			if tt.setup != nil {
				tt.setup(t, pkg)
			}

			results, err := checkDependencies(context.Background(), pkg, nil, nil)
			require.NoError(t, err)
			require.NotEmpty(t, results)
			statuses := make([]checkStatus, 0, len(results))
			for _, result := range results {
				statuses = append(statuses, result.Status)
				assert.False(t, result.CanFix)
			}

			assert.Contains(t, statuses, tt.want)
			if tt.want == checkPassed {
				assert.NotContains(t, statuses, checkFailed)
			}
		})
	}
}

func TestCheckDependenciesExternalPaths(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
	require.NoError(t, os.Mkdir(pkg.Vendor, 0o755))
	external := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(external, "library.inc"), []byte("user source"), 0o644))
	if err := os.Symlink(external, filepath.Join(pkg.Vendor, "repo")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	results, err := checkDependencies(context.Background(), pkg, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status)

	content, err := os.ReadFile(filepath.Join(external, "library.inc"))
	require.NoError(t, err)
	assert.Equal(t, "user source", string(content))
}

func TestCheckDependenciesIntegrityAndRevision(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
	path := filepath.Join(pkg.Vendor, "repo")
	commit := newCheckDependencyRepo(t, path, "")
	lf := lockfile.New("test")
	lf.AddDependency("github.com/owner/repo", lockfile.LockedDependency{
		User: "owner", Repo: "repo", Commit: commit, Integrity: lockfile.CalculateCommitIntegrity(commit),
	})

	results, err := checkDependencies(context.Background(), pkg, lf, nil)
	require.NoError(t, err)
	assert.Equal(t, checkPassed, results[0].Status)

	require.NoError(t, os.WriteFile(filepath.Join(path, "library.inc"), []byte("local change"), 0o644))

	results, err = checkDependencies(context.Background(), pkg, lf, nil)
	require.NoError(t, err)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Contains(t, results[0].Message, "integrity")

	content, err := os.ReadFile(filepath.Join(path, "library.inc"))
	require.NoError(t, err)
	assert.Equal(t, "local change", string(content))

	locked := lf.Dependencies["github.com/owner/repo"]
	locked.Commit = "1111111111111111111111111111111111111111"
	locked.Integrity = ""
	lf.Dependencies["github.com/owner/repo"] = locked

	results, err = checkDependencies(context.Background(), pkg, lf, nil)
	require.NoError(t, err)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Contains(t, results[0].Message, "revision")
}

func TestCheckDependenciesCyclesOverridesAndCancellation(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"old/repo"}
	newCheckDependencyRepo(t, filepath.Join(pkg.Vendor, "repo"), `{"dependencies":["old/repo"]}`)

	results, err := checkDependencies(context.Background(), pkg, nil, map[string]string{"old/repo": "new/repo"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = checkDependencies(ctx, pkg, nil, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCheckDependenciesMultipleFailures(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/first", "owner/second"}

	results, err := checkDependencies(context.Background(), pkg, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Equal(t, checkFailed, results[1].Status)
}

func TestCheckDependenciesDoesNotInitializeMissingSubmodules(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
	path := filepath.Join(pkg.Vendor, "repo")
	newCheckDependencyRepo(t, path, "")
	require.NoError(t, os.WriteFile(filepath.Join(path, ".gitmodules"), []byte("[submodule \"child\"]\n\tpath = child\n\turl = https://example.com/child.git\n"), 0o600))

	repo, err := git.PlainOpen(path)
	require.NoError(t, err)

	cfg, err := repo.Config()
	require.NoError(t, err)

	cfg.Submodules["child"] = &config.Submodule{Name: "child", Path: "child", URL: "https://example.com/child.git"}
	require.NoError(t, repo.SetConfig(cfg))

	before := snapshotCheckDependency(t, path)

	results, err := checkDependencies(t.Context(), pkg, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkFailed, results[0].Status)
	assert.Error(t, results[0].Err)
	assert.Equal(t, before, snapshotCheckDependency(t, path))
}

func TestCheckDependenciesSupportsLinkedWorktree(t *testing.T) {
	pkg := newCheckDependenciesPackage(t)
	pkg.Dependencies = []versioning.DependencyString{"owner/repo"}
	parent := filepath.Join(t.TempDir(), "parent")
	commit := newCheckDependencyRepo(t, parent, "")
	path := filepath.Join(pkg.Vendor, "repo")
	require.NoError(t, os.MkdirAll(path, 0o755))

	metadata := filepath.Join(parent, ".git", "worktrees", "linked")
	require.NoError(t, os.MkdirAll(metadata, 0o755))
	indexContents, err := os.ReadFile(filepath.Join(parent, ".git", "index"))
	require.NoError(t, err)
	for name, contents := range map[string][]byte{
		"HEAD": []byte(commit + "\n"), "commondir": []byte("../..\n"),
		"gitdir": []byte(filepath.Join(path, ".git") + "\n"), "index": indexContents,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(metadata, name), contents, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(path, ".git"), []byte("gitdir: "+metadata+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "library.inc"), []byte("stock Library() {}"), 0o644))

	lf := lockfile.New("test")
	lf.AddDependency("github.com/owner/repo", lockfile.LockedDependency{
		User: "owner", Repo: "repo", Commit: commit, Integrity: lockfile.CalculateCommitIntegrity(commit),
	})

	parentBefore := snapshotCheckDependency(t, parent)
	before := snapshotCheckDependency(t, path)

	results, err := checkDependencies(t.Context(), pkg, lf, nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, checkPassed, results[0].Status, "%v", results[0].Err)
	assert.Equal(t, parentBefore, snapshotCheckDependency(t, parent))
	assert.Equal(t, before, snapshotCheckDependency(t, path))
}

func snapshotCheckDependency(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		var hash [32]byte
		if info.Mode().IsRegular() {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			hash = sha256.Sum256(contents)
		}

		files[path] = fmt.Sprintf("%o:%d:%x", info.Mode(), info.ModTime().UnixNano(), hash)
		return nil
	}))

	return files
}

func newCheckDependenciesPackage(t *testing.T) pawnpackage.Package {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.Mkdir(root, 0o755))

	return pawnpackage.Package{Parent: true, LocalPath: root, Vendor: filepath.Join(root, "dependencies")}
}

func newCheckDependencyRepo(t *testing.T, path, definition string) string {
	t.Helper()
	repo, err := git.PlainInit(path, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(path, "library.inc"), []byte("stock Library() {}"), 0o644))

	wt, err := repo.Worktree()
	require.NoError(t, err)

	_, err = wt.Add("library.inc")
	require.NoError(t, err)
	if definition != "" {
		require.NoError(t, os.WriteFile(filepath.Join(path, "pawn.json"), []byte(definition), 0o644))
		_, err = wt.Add("pawn.json")
		require.NoError(t, err)
	}

	commit, err := wt.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "Fixture", Email: "fixture@example.com", When: time.Unix(0, 0)}})
	require.NoError(t, err)

	return commit.String()
}
