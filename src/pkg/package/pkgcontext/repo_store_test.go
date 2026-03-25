package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

type fakeRepositoryStore struct {
	openCalls  int
	cloneCalls int
	openFn     func(path string) (*git.Repository, error)
	cloneFn    func(path string, isBare bool, opts *git.CloneOptions) (*git.Repository, error)
}

type fakeRepositoryHealth struct {
	validateCalls int
	repairCalls   int
	validateFn    func(path string) (bool, error)
	repairFn      func(path string) error
}

func (f *fakeRepositoryStore) Open(path string) (*git.Repository, error) {
	f.openCalls++
	return f.openFn(path)
}

func (f *fakeRepositoryStore) Clone(path string, isBare bool, opts *git.CloneOptions) (*git.Repository, error) {
	f.cloneCalls++
	return f.cloneFn(path, isBare, opts)
}

func (f *fakeRepositoryHealth) Validate(path string) (bool, error) {
	f.validateCalls++
	return f.validateFn(path)
}

func (f *fakeRepositoryHealth) Repair(path string) error {
	f.repairCalls++
	return f.repairFn(path)
}

func TestEnsureRepoExistsUsesInjectedRepositoryStoreForClone(t *testing.T) {
	t.Parallel()

	store := &fakeRepositoryStore{}
	health := &fakeRepositoryHealth{
		validateFn: func(string) (bool, error) { return true, nil },
		repairFn:   func(string) error { return nil },
	}
	store.openFn = func(string) (*git.Repository, error) {
		return nil, git.ErrRepositoryNotExists
	}
	store.cloneFn = func(path string, _ bool, _ *git.CloneOptions) (*git.Repository, error) {
		repo, err := git.PlainInit(path, false)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(path, "README.md"), []byte("ok"), 0o644))
		wt, err := repo.Worktree()
		require.NoError(t, err)
		_, err = wt.Add("README.md")
		require.NoError(t, err)
		_, err = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com"}})
		require.NoError(t, err)

		return repo, nil
	}

	pcx := PackageContext{RepoStore: store, RepoHealth: health}
	to := filepath.Join(t.TempDir(), "repo")

	repo, err := pcx.ensureRepoExists(repoEnsureRequest{
		From:        "https://example.com/repo.git",
		To:          to,
		Branch:      "",
		SSH:         false,
		ForceUpdate: false,
	})
	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, 1, store.openCalls)
	assert.Equal(t, 1, store.cloneCalls)
	assert.Equal(t, 1, health.validateCalls)
	valid, err := ValidateRepository(to)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestEnsureDependencyRepositoryUsesInjectedRepositoryStoreForOpen(t *testing.T) {
	t.Parallel()

	depPath := t.TempDir()
	repo, err := git.PlainInit(depPath, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(depPath, "README.md"), []byte("ok"), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com"}})
	require.NoError(t, err)

	store := &fakeRepositoryStore{
		openFn: func(string) (*git.Repository, error) {
			return repo, nil
		},
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
			t.Fatal("clone should not be called when repository opens successfully")
			return nil, nil
		},
	}
	health := &fakeRepositoryHealth{
		validateFn: func(string) (bool, error) { return true, nil },
		repairFn:   func(string) error { return nil },
	}

	pcx := &PackageContext{RepoStore: store, RepoHealth: health}
	got, err := pcx.ensureDependencyRepository(versioning.DependencyMeta{User: "fixture", Repo: "repo"}, depPath)
	require.NoError(t, err)
	assert.Same(t, repo, got)
	assert.Equal(t, 1, store.openCalls)
	assert.Equal(t, 0, store.cloneCalls)
	assert.Equal(t, 0, health.validateCalls)
}

func TestEnsureRepoExistsUsesInjectedRepositoryHealth(t *testing.T) {
	t.Parallel()

	store := &fakeRepositoryStore{
		openFn: func(string) (*git.Repository, error) {
			return nil, git.ErrRepositoryNotExists
		},
		cloneFn: func(path string, _ bool, _ *git.CloneOptions) (*git.Repository, error) {
			repo, err := git.PlainInit(path, false)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(path, "README.md"), []byte("ok"), 0o644))
			wt, err := repo.Worktree()
			require.NoError(t, err)
			_, err = wt.Add("README.md")
			require.NoError(t, err)
			_, err = wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com"}})
			require.NoError(t, err)
			return repo, nil
		},
	}
	health := &fakeRepositoryHealth{
		validateFn: func(string) (bool, error) { return true, nil },
		repairFn:   func(string) error { return nil },
	}

	pcx := PackageContext{RepoStore: store, RepoHealth: health}
	_, err := pcx.ensureRepoExists(repoEnsureRequest{
		From:        "https://example.com/repo.git",
		To:          filepath.Join(t.TempDir(), "repo"),
		Branch:      "",
		SSH:         false,
		ForceUpdate: false,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, health.validateCalls)
	assert.Equal(t, 0, health.repairCalls)
}
