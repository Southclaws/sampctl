package pkgcontext

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildGitInfoResolvesPackedRefsInPathWithSpaces(t *testing.T) {
	repo, want := newBuildGitRepository(t, "feature/metadata")
	buildGitFixtureCommand(t, repo, "pack-refs", "--all")
	_, err := os.Stat(filepath.Join(repo, ".git", "refs", "heads", "feature", "metadata"))
	require.ErrorIs(t, err, os.ErrNotExist)

	commit, branch := readBuildGitInfo(t.Context(), repo)

	require.Equal(t, want, commit)
	require.Equal(t, "metadata", branch)
}

func TestBuildGitInfoResolvesLinkedWorktree(t *testing.T) {
	repo, want := newBuildGitRepository(t, "main")
	worktree := filepath.Join(t.TempDir(), "linked worktree")
	buildGitFixtureCommand(t, repo, "worktree", "add", "-b", "feature/linked", worktree)
	require.FileExists(t, filepath.Join(worktree, ".git"))

	commit, branch := readBuildGitInfo(t.Context(), worktree)

	require.Equal(t, want, commit)
	require.Equal(t, "linked", branch)
}

func TestBuildGitInfoResolvesDetachedShallowCheckout(t *testing.T) {
	repo, want := newBuildGitRepository(t, "main")
	checkout := filepath.Join(t.TempDir(), "shallow checkout")
	buildGitFixtureCommand(t, repo, "clone", "--no-local", "--depth=1", repo, checkout)
	buildGitFixtureCommand(t, checkout, "checkout", "--detach", want)

	commit, branch := readBuildGitInfo(t.Context(), checkout)

	require.Equal(t, want, commit)
	require.Empty(t, branch)
}

func TestBuildGitInfoDoesNotBorrowEnclosingRepository(t *testing.T) {
	repo, _ := newBuildGitRepository(t, "main")
	nested := filepath.Join(repo, "nested")
	require.NoError(t, os.Mkdir(nested, 0o700))

	commit, branch := readBuildGitInfo(t.Context(), nested)

	require.Empty(t, commit)
	require.Empty(t, branch)
}

func TestBuildGitInfoIgnoresGitRepositoryEnvironment(t *testing.T) {
	repo, want := newBuildGitRepository(t, "main")
	other, _ := newBuildGitRepository(t, "other")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)

	commit, branch := readBuildGitInfo(t.Context(), repo)

	require.Equal(t, want, commit)
	require.Equal(t, "main", branch)
}

func TestBuildGitInfoFallsBackToLooseRefWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	const want = "0123456789abcdef0123456789abcdef01234567"
	ref := filepath.Join(gitDir, "refs", "heads", "feature", "fallback")
	require.NoError(t, os.MkdirAll(filepath.Dir(ref), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/fallback\n"), 0o600))
	require.NoError(t, os.WriteFile(ref, []byte(want+"\n"), 0o600))

	commit, branch := readBuildGitInfo(t.Context(), repo)

	require.Equal(t, want, commit)
	require.Equal(t, "fallback", branch)
}

func TestBuildGitInfoOmitsMalformedDetachedCommit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("not-a-commit\n"), 0o600))

	commit, branch := readBuildGitInfo(t.Context(), repo)

	require.Empty(t, commit)
	require.Empty(t, branch)
}

func newBuildGitRepository(t *testing.T, branch string) (dir, commit string) {
	t.Helper()

	dir = filepath.Join(t.TempDir(), "project with spaces")
	require.NoError(t, os.Mkdir(dir, 0o700))
	buildGitFixtureCommand(t, dir, "init", "--initial-branch="+branch)
	buildGitFixtureCommand(t, dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "fixture")
	return dir, buildGitFixtureCommand(t, dir, "rev-parse", "HEAD")
}

func buildGitFixtureCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("Git is unavailable: %v", err)
	}
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, output)
	return strings.TrimSpace(string(output))
}
