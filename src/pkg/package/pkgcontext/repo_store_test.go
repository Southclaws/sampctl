package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
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

type constructorRuntimeEnvironment struct{}

type constructorRuntimeProvisioner struct{}

func (constructorRuntimeEnvironment) Run(context.Context, runtimecfg.Runtime, runtimepkg.RunOptions) error {
	return nil
}

func (constructorRuntimeEnvironment) PrepareRuntimeDirectory(string, string, string, string) error {
	return nil
}

func (constructorRuntimeEnvironment) CopyFileToRuntime(string, string, string) error {
	return nil
}

func (constructorRuntimeEnvironment) Ensure(context.Context, *github.Client, *runtimecfg.Runtime, bool) error {
	return nil
}

func (constructorRuntimeEnvironment) GenerateConfig(*runtimecfg.Runtime) error {
	return nil
}

func (constructorRuntimeProvisioner) EnsurePackageLayout(string, bool) error {
	return nil
}

func (constructorRuntimeProvisioner) EnsureBinaries(context.Context, string, runtimecfg.Runtime) (*runtimepkg.RuntimeManifestInfo, error) {
	return nil, nil
}

func (constructorRuntimeProvisioner) EnsurePlugins(runtimepkg.EnsurePluginsRequest) error {
	return nil
}

type fakeTransportAuth struct{}

func (fakeTransportAuth) Name() string {
	return "fake"
}

func (fakeTransportAuth) String() string {
	return "fake"
}

func (fakeTransportAuth) SetAuth(*transport.Endpoint) error {
	return nil
}

type constructorRemoteFetcher struct{}

func (constructorRemoteFetcher) Fetch(context.Context, versioning.DependencyMeta) (pawnpackage.Package, error) {
	return pawnpackage.Package{}, nil
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

	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store, RepoHealth: health}}
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

	pcx := &PackageContext{PackageServices: PackageServices{RepoStore: store, RepoHealth: health}}
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

	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store, RepoHealth: health}}
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

func TestNewPackageContextBaseUsesInjectedDependencies(t *testing.T) {
	t.Parallel()

	store := &fakeRepositoryStore{}
	health := &fakeRepositoryHealth{}
	fetcher := constructorRemoteFetcher{}
	runtimeEnv := constructorRuntimeEnvironment{}
	runtimeProv := constructorRuntimeProvisioner{}
	auth := fakeTransportAuth{}
	gh := github.NewClient(nil)

	pcx := newPackageContextBase(NewPackageContextOptions{
		GitHub:         gh,
		Auth:           auth,
		Platform:       "linux",
		CacheDir:       "/tmp/cache",
		RemotePackages: fetcher,
		RepoStore:      store,
		RepoHealth:     health,
		RuntimeEnv:     runtimeEnv,
		RuntimeProv:    runtimeProv,
	})

	assert.Same(t, gh, pcx.GitHub)
	assert.Same(t, store, pcx.RepoStore)
	assert.Same(t, health, pcx.RepoHealth)
	assert.Equal(t, fetcher, pcx.RemotePackages)
	assert.Equal(t, runtimeEnv, pcx.RuntimeEnv)
	assert.Equal(t, runtimeProv, pcx.RuntimeProv)
	assert.Equal(t, auth, pcx.GitAuth)
	assert.Equal(t, "linux", pcx.Platform)
	assert.Equal(t, "/tmp/cache", pcx.CacheDir)
}

func TestNewPackageContextBaseUsesDefaultsWhenDependenciesMissing(t *testing.T) {
	t.Parallel()

	pcx := newPackageContextBase(NewPackageContextOptions{})

	require.NotNil(t, pcx.RemotePackages)
	assert.IsType(t, GitRepositoryStore{}, pcx.RepoStore)
	assert.IsType(t, GitRepositoryHealth{}, pcx.RepoHealth)
	assert.IsType(t, runtimeEnvironmentAdapter{}, pcx.RuntimeEnv)
	assert.IsType(t, runtimeProvisionerAdapter{}, pcx.RuntimeProv)
	assert.Nil(t, pcx.GitHub)
	assert.Nil(t, pcx.GitAuth)
}

func TestPackageContextStatePromotion(t *testing.T) {
	t.Parallel()

	pcx := PackageContext{
		PackageServices: PackageServices{
			Platform: "linux",
			CacheDir: "/tmp/cache",
		},
		PackageResolvedState: PackageResolvedState{
			AllDependencies: []versioning.DependencyMeta{{User: "fixture", Repo: "repo"}},
		},
		PackageExecutionState: PackageExecutionState{
			BuildName:  "default",
			ForceBuild: true,
		},
		PackageLockfileState: PackageLockfileState{
			UseLockfile: true,
		},
	}

	assert.Equal(t, "linux", pcx.Platform)
	assert.Equal(t, "/tmp/cache", pcx.CacheDir)
	assert.Len(t, pcx.AllDependencies, 1)
	assert.Equal(t, "default", pcx.BuildName)
	assert.True(t, pcx.ForceBuild)
	assert.True(t, pcx.UseLockfile)
}
