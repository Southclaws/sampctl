package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

func TestIntegrityHelpers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.inc"), []byte("one"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("ignored"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden.inc"), []byte("ignored"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("ignored"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "c.pwn"), []byte("two"), 0o644))

	hash, err := CalculateDirectoryIntegrity(dir)
	require.NoError(t, err)
	assert.Contains(t, hash, IntegrityPrefix)

	ok, err := VerifyIntegrity(dir, hash)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = VerifyIntegrity(dir, IntegrityPrefix+"deadbeef")
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = VerifyIntegrity(dir, "")
	require.NoError(t, err)
	assert.True(t, ok)

	_, err = CalculateDirectoryIntegrity(filepath.Join(dir, "missing"))
	require.Error(t, err)

	assert.Equal(t, "commit:abcdef", CalculateCommitIntegrity("abcdef"))
	assert.Equal(t, "", CalculateCommitIntegrity(""))
	assert.True(t, isRelevantExtension(".inc"))
	assert.False(t, isRelevantExtension(".txt"))
	assert.Equal(t, "sha256", func() string { k, _ := ParseIntegrity(IntegrityPrefix + "abc"); return k }())
	assert.True(t, IsValidIntegrity(IntegrityPrefix+"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
	assert.True(t, IsValidIntegrity("commit:0123456789012345678901234567890123456789"))
	assert.False(t, IsValidIntegrity("sha256:short"))
	assert.False(t, IsValidIntegrity("unknown:value"))
	assert.False(t, IsValidIntegrity("malformed"))
}

func TestLockfileIOHelpers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	assert.Empty(t, GetPath(dir))
	require.Error(t, Save(dir, nil))

	invalid := filepath.Join(dir, Filename)
	require.NoError(t, os.WriteFile(invalid, []byte("{"), 0o644))
	_, err := Load(dir)
	require.Error(t, err)

	require.NoError(t, os.WriteFile(invalid, []byte(`{"version":0}`), 0o644))
	_, err = Load(dir)
	require.Error(t, err)
}

func TestResolverLifecycle(t *testing.T) {
	dir := t.TempDir()
	resolver, err := NewResolver(dir, "1.0.0", false)
	require.NoError(t, err)
	assert.Nil(t, resolver.GetLockfile())
	assert.False(t, resolver.HasLockfile())

	resolver, err = NewResolver(dir, "1.0.0", true)
	require.NoError(t, err)
	require.NotNil(t, resolver.GetLockfile())
	assert.False(t, resolver.HasLockfile())

	meta := versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.2.3"}
	_, commit := seedGitRepo(t, map[string]string{"pawn.json": "{}"}, "1.2.3")

	require.NoError(t, resolver.RecordResolution(meta, DependencyResolution{Commit: commit, Resolved: "1.2.3"}, true, "github.com/root/pkg"))
	assert.True(t, resolver.IsLocked(meta))
	assert.True(t, resolver.HasLockfile())

	lockedMeta := resolver.GetLockedVersion(meta)
	assert.Equal(t, commit, lockedMeta.Commit)
	assert.Empty(t, lockedMeta.Tag)

	lf := resolver.GetLockfile()
	dep, ok := lf.GetDependency(DependencyKey(meta))
	require.True(t, ok)
	assert.Equal(t, ":1.2.3", dep.Constraint)
	assert.Equal(t, "1.2.3", dep.Resolved)
	assert.Equal(t, []string{"github.com/root/pkg"}, dep.RequiredBy)
	assert.True(t, dep.Transitive)
	assert.Equal(t, CalculateCommitIntegrity(commit), dep.Integrity)

	resolver.PruneMissing(nil)
	assert.False(t, resolver.IsLocked(meta))

	localMeta := versioning.DependencyMeta{Scheme: "plugin", Local: "plugins/test"}
	require.NoError(t, resolver.RecordLocalDependency(localMeta))
	assert.True(t, resolver.IsLocked(localMeta))

	resolver.RecordRuntime("0.3.7", "linux", "samp", []LockedFileInfo{{Path: "server", Size: 1}})
	resolver.RecordBuild(BuildRecord{
		CompilerVersion: "3.10.11",
		CompilerPreset:  "pawn-lang",
		Entry:           "main.pwn",
		Output:          "main.amx",
		OutputHash:      "sha256:abc",
	})
	require.NoError(t, resolver.Save())
	assert.True(t, Exists(dir))

	loaded, err := Load(dir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.True(t, loaded.HasRuntime())
	assert.True(t, loaded.HasBuild())

	resolver.ForceUpdate()
	assert.False(t, resolver.HasLockfile())
	assert.Empty(t, resolver.GetLockfile().Dependencies)
	assert.True(t, resolver.modified)
}

func TestDefaultResolvedVersionFallbacks(t *testing.T) {
	t.Run("uses tag", func(t *testing.T) {
		assert.Equal(t, "1.2.3", defaultResolvedVersion(versioning.DependencyMeta{User: "u", Repo: "r", Tag: "1.2.3"}, "abcdef"))
	})

	t.Run("falls back to branch", func(t *testing.T) {
		assert.Equal(t, "main", defaultResolvedVersion(versioning.DependencyMeta{User: "u", Repo: "r", Branch: "main"}, "abcdef"))
	})

	t.Run("falls back to commit prefix", func(t *testing.T) {
		commit := "1234567890abcdef"
		assert.Equal(t, commit[:8], defaultResolvedVersion(versioning.DependencyMeta{User: "u", Repo: "r", Commit: commit}, commit))
	})

	t.Run("defaults to HEAD", func(t *testing.T) {
		assert.Equal(t, "HEAD", defaultResolvedVersion(versioning.DependencyMeta{User: "u", Repo: "r"}, "abcdef"))
	})
}

func seedGitRepo(t *testing.T, files map[string]string, tag string) (*git.Repository, string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	for name, body := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		_, err = wt.Add(name)
		require.NoError(t, err)
	}

	hash, err := wt.Commit("fixture", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	require.NoError(t, err)

	if tag != "" {
		_, err = repo.CreateTag(tag, hash, nil)
		require.NoError(t, err)
	}

	return repo, hash.String()
}
