package lockfile

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitIntegrityDoesNotInitializeMissingSubmodules(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "missing module"
		if existing {
			name = "missing module HEAD"
		}
		t.Run(name, func(t *testing.T) {
			repo, commit := seedGitRepo(t, map[string]string{
				"main.pwn":    "main() {}",
				".gitmodules": "[submodule \"child\"]\n\tpath = child\n\turl = https://example.com/child.git\n",
			}, "")
			wt, err := repo.Worktree()
			require.NoError(t, err)

			dir := wt.Filesystem.Root()
			registerIntegritySubmodule(t, repo)
			if existing {
				module := filepath.Join(dir, ".git", "modules", "child")
				require.NoError(t, os.MkdirAll(module, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(module, "marker"), []byte("user data"), 0o600))
			}

			before := snapshotIntegrityFiles(t, dir)
			ok, err := VerifyIntegrityContext(t.Context(), dir, CalculateCommitIntegrity(commit))
			assert.Error(t, err)
			assert.False(t, ok)
			assert.Equal(t, before, snapshotIntegrityFiles(t, dir), "integrity verification must not initialize submodule files")
		})
	}
}

func TestCommitIntegritySupportsInitializedSubmodules(t *testing.T) {
	repo, _ := seedGitRepo(t, map[string]string{
		"main.pwn":           "main() {}",
		"vendor/library.inc": "stock Vendored() {}",
		".gitmodules":        "[submodule \"child\"]\n\tpath = child\n\turl = https://example.com/child.git\n[submodule \"vendor\"]\n\tpath = vendor\n\turl = https://example.com/vendor.git\n",
	}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)

	dir := wt.Filesystem.Root()

	child, childCommit := seedGitRepo(t, map[string]string{"library.inc": "stock Library() {}"}, "")
	childWorktree, err := child.Worktree()
	require.NoError(t, err)

	childPath := filepath.Join(dir, "child")
	require.NoError(t, os.Rename(childWorktree.Filesystem.Root(), childPath))

	module := filepath.Join(dir, ".git", "modules", "child")
	require.NoError(t, os.MkdirAll(filepath.Dir(module), 0o755))
	require.NoError(t, os.Rename(filepath.Join(childPath, ".git"), module))
	require.NoError(t, os.WriteFile(filepath.Join(childPath, ".git"), []byte("gitdir: "+module+"\n"), 0o600))
	registerIntegritySubmodule(t, repo)

	idx, err := repo.Storer.Index()
	require.NoError(t, err)
	entry := idx.Add("child")
	entry.Hash, entry.Mode = plumbing.NewHash(childCommit), filemode.Submodule
	require.NoError(t, repo.Storer.SetIndex(idx))

	commit, err := wt.Commit("add submodule", &git.CommitOptions{Author: &object.Signature{
		Name: "test", Email: "test@example.com", When: time.Unix(200, 0),
	}})
	require.NoError(t, err)

	before := snapshotIntegrityFiles(t, dir)
	ok, err := VerifyIntegrityContext(t.Context(), dir, CalculateCommitIntegrity(commit.String()))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, before, snapshotIntegrityFiles(t, dir))

	child, err = git.PlainOpen(childPath)
	require.NoError(t, err)
	childWorktree, err = child.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(childPath, "library.inc"), []byte("stock Changed() {}"), 0o644))
	_, err = childWorktree.Add("library.inc")
	require.NoError(t, err)
	_, err = childWorktree.Commit("change submodule", &git.CommitOptions{Author: &object.Signature{
		Name: "test", Email: "test@example.com", When: time.Unix(300, 0),
	}})
	require.NoError(t, err)

	before = snapshotIntegrityFiles(t, dir)
	ok, err = VerifyIntegrityContext(t.Context(), dir, CalculateCommitIntegrity(commit.String()))
	require.NoError(t, err)
	assert.False(t, ok, "obsolete declarations must not hide a changed real gitlink")
	assert.Equal(t, before, snapshotIntegrityFiles(t, dir))
}

func TestCommitIntegrityLeavesUnregisteredSubmodulesAlone(t *testing.T) {
	repo, commit := seedGitRepo(t, map[string]string{
		"main.pwn":    "main() {}",
		".gitmodules": "[submodule \"child\"]\n\tpath = child\n\turl = https://example.com/child.git\n",
	}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)

	dir := wt.Filesystem.Root()
	before := snapshotIntegrityFiles(t, dir)

	ok, err := VerifyIntegrityContext(t.Context(), dir, CalculateCommitIntegrity(commit))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, before, snapshotIntegrityFiles(t, dir))
}

func TestRepositoryStatusDoesNotReinitializeRemovedModuleHEAD(t *testing.T) {
	repo, _ := seedGitRepo(t, map[string]string{
		"main.pwn":    "main() {}",
		".gitmodules": "[submodule \"child\"]\n\tpath = child\n\turl = https://example.com/child.git\n",
	}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)

	dir := wt.Filesystem.Root()
	registerIntegritySubmodule(t, repo)
	module := filepath.Join(dir, ".git", "modules", "child")
	_, err = git.PlainInit(module, true)
	require.NoError(t, err)

	checked, err := OpenRepositoryContext(t.Context(), dir)
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(module, "HEAD")))

	before := snapshotIntegrityFiles(t, dir)
	checkedWorktree, err := checked.Worktree()
	require.NoError(t, err)

	_, err = checkedWorktree.Status()
	assert.Error(t, err)
	assert.Equal(t, before, snapshotIntegrityFiles(t, dir))
}

func TestCommitIntegritySupportsLinkedWorktree(t *testing.T) {
	repo, commit := seedGitRepo(t, map[string]string{"main.pwn": "main() {}"}, "")
	wt, err := repo.Worktree()
	require.NoError(t, err)

	parent := wt.Filesystem.Root()
	linked := seedIntegrityLinkedWorktree(t, parent, commit)
	before := snapshotIntegrityFiles(t, parent)
	linkedBefore := snapshotIntegrityFiles(t, linked)

	ok, err := VerifyIntegrityContext(t.Context(), linked, CalculateCommitIntegrity(commit))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, before, snapshotIntegrityFiles(t, parent))
	assert.Equal(t, linkedBefore, snapshotIntegrityFiles(t, linked))
}

func TestGitMetadataPreflightRejectsInvalidCommonDirectory(t *testing.T) {
	for _, contents := range []string{"", "missing\n"} {
		t.Run(contents, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "commondir"), []byte(contents), 0o600))

			require.Error(t, ValidateWorktreeFilesContext(t.Context(), dir))
		})
	}
}

func registerIntegritySubmodule(t *testing.T, repo *git.Repository) {
	t.Helper()

	cfg, err := repo.Config()
	require.NoError(t, err)

	cfg.Submodules["child"] = &config.Submodule{Name: "child", Path: "child", URL: "https://example.com/child.git"}
	require.NoError(t, repo.SetConfig(cfg))
}

func seedIntegrityLinkedWorktree(t *testing.T, parent, commit string) string {
	t.Helper()

	linked := t.TempDir()
	metadata := filepath.Join(parent, ".git", "worktrees", "linked")
	require.NoError(t, os.MkdirAll(metadata, 0o755))

	indexContents, err := os.ReadFile(filepath.Join(parent, ".git", "index"))
	require.NoError(t, err)

	for name, contents := range map[string][]byte{
		"HEAD": []byte(commit + "\n"), "commondir": []byte("../..\n"),
		"gitdir": []byte(filepath.Join(linked, ".git") + "\n"), "index": indexContents,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(metadata, name), contents, 0o600))
	}

	require.NoError(t, os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: "+metadata+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(linked, "main.pwn"), []byte("main() {}"), 0o644))

	return linked
}

type integrityFileState struct {
	Mode     os.FileMode
	Modified int64
	Hash     [32]byte
}

func snapshotIntegrityFiles(t *testing.T, dir string) map[string]integrityFileState {
	t.Helper()

	files := make(map[string]integrityFileState)
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		state := integrityFileState{Mode: info.Mode(), Modified: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			state.Hash = sha256.Sum256(contents)
		}

		files[path] = state
		return nil
	}))

	return files
}
