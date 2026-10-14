package pkgcontext

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-github/github"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime"
)

type failingLifecycleLock struct {
	fakeDependencyLock
	err error
}

func (f *failingLifecycleLock) Save() error { return f.err }

func lifecycleContext(t *testing.T, format string) *PackageContext {
	t.Helper()
	dir := t.TempDir()
	pkg := pawnpackage.Package{Format: format, LocalPath: dir, Parent: true, User: "fixture", Repo: "root"}
	require.NoError(t, pkg.WriteDefinition())

	return &PackageContext{Package: pkg, PackageServices: PackageServices{CacheDir: t.TempDir(), Platform: "linux", RuntimeProv: &fakeRuntimeProvisioner{}}}
}

func TestInstallFailureRestoresDefinitionAndMemory(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			before, err := os.ReadFile(filepath.Join(pcx.Package.LocalPath, "pawn."+format))
			require.NoError(t, err)

			require.Error(t, pcx.Install(context.Background(), []versioning.DependencyString{"includes://local/includes", "invalid"}, false))
			assert.Empty(t, pcx.Package.Dependencies)

			after, err := os.ReadFile(filepath.Join(pcx.Package.LocalPath, "pawn."+format))
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestInstallLockfileFailureRestoresInstalledState(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "1.0.0"}
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0"})

			sentinel := errors.New("lock persistence failed")
			pcx.SetLockfileResolver(&failingLifecycleLock{fakeDependencyLock: fakeDependencyLock{lockfile: lockfile.New("test")}, err: sentinel})

			manifest := filepath.Join(pcx.Package.LocalPath, "pawn."+format)
			before, err := os.ReadFile(manifest)
			require.NoError(t, err)

			err = pcx.Install(context.Background(), []versioning.DependencyString{"fixture/dep:1.0.0"}, false)
			require.ErrorIs(t, err, sentinel)
			assert.Empty(t, pcx.Package.Dependencies)

			after, err := os.ReadFile(manifest)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			assert.NoDirExists(t, filepath.Join(pcx.Package.LocalPath, "dependencies", "dep"))
			assert.DirExists(t, meta.CachePath(pcx.CacheDir))
			assert.NoFileExists(t, filepath.Join(pcx.Package.LocalPath, lockfile.Filename))
		})
	}
}

func TestInstallKeepsExistingPinsAndAddsAllTargets(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	for _, name := range []string{"one", "two"} {
		seedEnsureProjectDependencyRepo(t, pcx.CacheDir, versioning.DependencyMeta{User: "fixture", Repo: name}, []string{"1.0.0", "2.0.0"})
	}

	require.NoError(t, pcx.InitLockfileResolver("test"))

	require.NoError(t, pcx.Install(context.Background(), []versioning.DependencyString{"fixture/one:1.0.0"}, false))

	lf, err := lockfile.Load(pcx.Package.LocalPath)
	require.NoError(t, err)
	original := lf.Dependencies["github.com/fixture/one"].Commit

	require.NoError(t, pcx.Install(context.Background(), []versioning.DependencyString{"fixture/one:1.0.0", "fixture/two:1.0.0"}, false))
	assert.Len(t, pcx.Package.Dependencies, 2)

	lf, err = lockfile.Load(pcx.Package.LocalPath)
	require.NoError(t, err)
	assert.Equal(t, original, lf.Dependencies["github.com/fixture/one"].Commit)
	assert.Contains(t, lf.Dependencies, "github.com/fixture/two")
}

func TestInstallBareDeclaredDependencyPreservesPinWithoutTagLookup(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "1.0.0"}
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})

			require.NoError(t, pcx.InitLockfileResolver("test"))

			require.NoError(t, pcx.Install(context.Background(), []versioning.DependencyString{"fixture/dep:1.0.0"}, false))

			manifestPath := filepath.Join(pcx.Package.LocalPath, "pawn."+format)
			manifest, err := os.ReadFile(manifestPath)
			require.NoError(t, err)

			lockPath := filepath.Join(pcx.Package.LocalPath, lockfile.Filename)
			pinned, err := os.ReadFile(lockPath)
			require.NoError(t, err)

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(`[{"name":"2.0.0"}]`))
			}))
			t.Cleanup(server.Close)

			pcx.GitHub = github.NewClient(server.Client())
			pcx.GitHub.BaseURL, err = url.Parse(server.URL + "/")
			require.NoError(t, err)

			require.NoError(t, pcx.Install(context.Background(), []versioning.DependencyString{"fixture/dep"}, false))
			assert.Zero(t, requests.Load())

			afterManifest, err := os.ReadFile(manifestPath)
			require.NoError(t, err)
			assert.Equal(t, manifest, afterManifest)

			afterLock, err := os.ReadFile(lockPath)
			require.NoError(t, err)
			assert.Equal(t, pinned, afterLock)
			assert.Equal(t, []versioning.DependencyString{"fixture/dep:1.0.0"}, pcx.Package.Dependencies)
		})
	}
}

func TestUninstallSynchronizesExistingLockfileOffline(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/one:1.0.0", "includes://local/includes"}
			require.NoError(t, pcx.Package.WriteDefinition())

			lf := lockfile.New("test")
			lf.Dependencies["github.com/fixture/one"] = lockfile.LockedDependency{User: "fixture", Repo: "one", Constraint: ":1.0.0", Commit: "0123456789012345678901234567890123456789"}
			lf.Dependencies["includes://local/includes"] = lockfile.LockedDependency{Scheme: "includes", Local: "includes"}
			require.NoError(t, lockfile.Save(pcx.Package.LocalPath, lf))

			installed := filepath.Join(pcx.Package.LocalPath, "dependencies", "one")
			require.NoError(t, os.MkdirAll(installed, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(installed, "keep.inc"), []byte("user data"), 0o644))

			require.NoError(t, pcx.Uninstall([]versioning.DependencyString{"fixture/one:1.0.0"}, false))

			lf, err := lockfile.Load(pcx.Package.LocalPath)
			require.NoError(t, err)
			assert.NotContains(t, lf.Dependencies, "github.com/fixture/one")
			assert.Contains(t, lf.Dependencies, "includes://local/includes")
			assert.FileExists(t, filepath.Join(installed, "keep.inc"))
		})
	}
}

func TestUninstallPreservesSharedTransitivePins(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/a:1.0.0", "fixture/b:1.0.0", "fixture/shared:1.0.0"}
	require.NoError(t, pcx.Package.WriteDefinition())

	lf := lockfile.New("test")
	for _, name := range []string{"a", "b", "shared", "unused"} {
		lf.Dependencies["github.com/fixture/"+name] = lockfile.LockedDependency{User: "fixture", Repo: name, Commit: "0123456789012345678901234567890123456789", Constraint: ":1.0.0"}
	}

	shared := lf.Dependencies["github.com/fixture/shared"]
	shared.RequiredBy = []string{"github.com/fixture/a", "github.com/fixture/b"}
	lf.Dependencies["github.com/fixture/shared"] = shared

	unused := lf.Dependencies["github.com/fixture/unused"]
	unused.Transitive = true
	unused.RequiredBy = []string{"github.com/fixture/a"}
	lf.Dependencies["github.com/fixture/unused"] = unused
	require.NoError(t, lockfile.Save(pcx.Package.LocalPath, lf))

	require.NoError(t, pcx.Uninstall([]versioning.DependencyString{"fixture/a:1.0.0", "fixture/shared:1.0.0"}, false))

	got, err := lockfile.Load(pcx.Package.LocalPath)
	require.NoError(t, err)
	assert.NotContains(t, got.Dependencies, "github.com/fixture/a")
	assert.NotContains(t, got.Dependencies, "github.com/fixture/unused")
	assert.Equal(t, shared.Commit, got.Dependencies["github.com/fixture/shared"].Commit)
	assert.True(t, got.Dependencies["github.com/fixture/shared"].Transitive)
	assert.Equal(t, []string{"github.com/fixture/b"}, got.Dependencies["github.com/fixture/shared"].RequiredBy)
}

func TestDependencyOperationRejectsConcurrentProjectMutation(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	first, err := pcx.beginDependencyOperation(context.Background())
	require.NoError(t, err)

	second := *pcx
	_, err = second.beginDependencyOperation(context.Background())
	require.Error(t, err)

	require.NoError(t, pcx.finishDependencyOperation(first, nil))

	require.NoError(t, pcx.Install(context.Background(), nil, false))
}

func TestDependencyProjectRollbackReportsPersistenceFailure(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	state, err := pcx.beginDependencyOperation(context.Background())
	require.NoError(t, err)

	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep:1.0.0"}
	require.NoError(t, os.Remove(state.manifest.path))
	require.NoError(t, os.Mkdir(state.manifest.path, 0o755))

	sentinel := errors.New("installation failed")

	err = pcx.finishDependencyOperation(state, sentinel)
	require.ErrorIs(t, err, sentinel)
	var operationErr *OperationError
	require.ErrorAs(t, err, &operationErr)
	assert.Equal(t, "rollback", operationErr.Operation)
	assert.Empty(t, pcx.Package.Dependencies)
	assert.DirExists(t, state.manifest.path)
}

func TestEnsureFailureRestoresUpdatedManifestAndExistingInstallation(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			meta := versioning.DependencyMeta{User: "fixture", Repo: "dep"}
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep:1.0.0"}
			require.NoError(t, pcx.Package.WriteDefinition())

			require.NoError(t, pcx.EnsureDependenciesCached())

			require.NoError(t, pcx.InitLockfileResolver("test"))

			_, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{})
			require.NoError(t, err)

			manifestPath := filepath.Join(pcx.Package.LocalPath, "pawn."+format)
			originalManifest, err := os.ReadFile(manifestPath)
			require.NoError(t, err)

			originalLock, err := os.ReadFile(filepath.Join(pcx.Package.LocalPath, lockfile.Filename))
			require.NoError(t, err)

			installedPath := filepath.Join(pcx.Package.LocalPath, "dependencies", "dep", "dep.pwn")
			originalDependency, err := os.ReadFile(installedPath)
			require.NoError(t, err)

			sentinel := errors.New("runtime failed after dependency replacement")
			pcx.RuntimeProv = &fakeRuntimeProvisioner{binariesErr: sentinel}

			updated, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{Enabled: true, Force: true})
			require.ErrorIs(t, err, sentinel)
			assert.False(t, updated)

			manifest, err := os.ReadFile(manifestPath)
			require.NoError(t, err)
			assert.Equal(t, originalManifest, manifest)

			pinned, err := os.ReadFile(filepath.Join(pcx.Package.LocalPath, lockfile.Filename))
			require.NoError(t, err)
			assert.Equal(t, originalLock, pinned)

			dependency, err := os.ReadFile(installedPath)
			require.NoError(t, err)
			assert.Equal(t, originalDependency, dependency)
			assert.Equal(t, []versioning.DependencyString{"fixture/dep:1.0.0"}, pcx.Package.Dependencies)
		})
	}
}

func TestEnsureRepeatedDoesNotRewritePins(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, versioning.DependencyMeta{User: "fixture", Repo: "dep"}, []string{"1.0.0"})
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep:1.0.0"}
	require.NoError(t, pcx.Package.WriteDefinition())

	require.NoError(t, pcx.EnsureDependenciesCached())

	require.NoError(t, pcx.InitLockfileResolver("test"))

	_, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{})
	require.NoError(t, err)

	lockPath := filepath.Join(pcx.Package.LocalPath, lockfile.Filename)
	before, err := os.ReadFile(lockPath)
	require.NoError(t, err)

	_, err = pcx.EnsureProject(context.Background(), DependencyUpdateRequest{})
	require.NoError(t, err)

	after, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestEnsureWithoutLockfileSupportIgnoresLockfilePath(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			pcx := lifecycleContext(t, "json")
			lockPath := filepath.Join(pcx.Package.LocalPath, lockfile.Filename)
			require.NoError(t, os.Mkdir(lockPath, 0o755))
			marker := filepath.Join(lockPath, "keep")
			require.NoError(t, os.WriteFile(marker, []byte("user data"), 0o644))

			sentinel := errors.New("runtime unavailable")
			if fail {
				pcx.RuntimeProv = &fakeRuntimeProvisioner{binariesErr: sentinel}
			}

			_, err := pcx.EnsureProject(context.Background(), DependencyUpdateRequest{})
			if fail {
				require.ErrorIs(t, err, sentinel)
			} else {
				require.NoError(t, err)
			}

			contents, err := os.ReadFile(marker)
			require.NoError(t, err)
			assert.Equal(t, "user data", string(contents))
		})
	}
}

type lifecycleCallbackProvisioner struct {
	fakeRuntimeProvisioner
	callback func()
}

func (provisioner *lifecycleCallbackProvisioner) EnsurePlugins(_ runtime.EnsurePluginsRequest) error {
	provisioner.callback()
	return nil
}

func TestInstallManifestPersistenceFailureReportsIncompleteRollback(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			seedEnsureProjectDependencyRepo(t, pcx.CacheDir, versioning.DependencyMeta{User: "fixture", Repo: "dep"}, []string{"1.0.0"})

			manifest := filepath.Join(pcx.Package.LocalPath, "pawn."+format)
			before, err := os.ReadFile(manifest)
			require.NoError(t, err)

			pcx.RuntimeProv = &lifecycleCallbackProvisioner{callback: func() {
				require.NoError(t, os.Rename(manifest, manifest+".saved"))
				require.NoError(t, os.Mkdir(manifest, 0o755))
			}}

			err = pcx.Install(context.Background(), []versioning.DependencyString{"fixture/dep:1.0.0"}, false)
			require.Error(t, err)
			var persistence *OperationError
			require.ErrorAs(t, err, &persistence)
			assert.Equal(t, "manifest", persistence.Operation)
			assert.Equal(t, OutcomeRollbackIncomplete, persistence.Outcome)
			require.Contains(t, err.Error(), "rollback failed")

			original, readErr := os.ReadFile(manifest + ".saved")
			require.NoError(t, readErr)
			assert.Equal(t, before, original)
			assert.Empty(t, pcx.Package.Dependencies)
			assert.DirExists(t, filepath.Join(pcx.Package.LocalPath, "dependencies", "dep"))
			assert.FileExists(t, filepath.Join(pcx.Package.LocalPath, DependencyRecoveryFilename))
			assert.DirExists(t, manifest)
		})
	}
}

func TestInstallCancellationAfterPromotionRestoresProject(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, versioning.DependencyMeta{User: "fixture", Repo: "dep"}, []string{"1.0.0"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pcx.RuntimeProv = &lifecycleCallbackProvisioner{callback: cancel}

	err := pcx.Install(ctx, []versioning.DependencyString{"fixture/dep:1.0.0"}, false)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, pcx.Package.Dependencies)
	assert.NoDirExists(t, filepath.Join(pcx.Package.LocalPath, "dependencies", "dep"))
	assert.NoFileExists(t, filepath.Join(pcx.Package.LocalPath, lockfile.Filename))

	pkg, err := pawnpackage.PackageFromDir(pcx.Package.LocalPath)
	require.NoError(t, err)
	assert.Empty(t, pkg.Dependencies)
}

func TestForcedEnsureFailurePreservesPinsOnNextEnsure(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dep"}
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep"}
	require.NoError(t, pcx.Package.WriteDefinition())

	require.NoError(t, pcx.EnsureDependenciesCached())

	require.NoError(t, pcx.InitLockfileResolver("test"))

	lf := pcx.GetLockfile()
	repo, err := pcx.repositoryStore().Open(meta.CachePath(pcx.CacheDir))
	require.NoError(t, err)

	reference, err := versioning.RefFromTag(repo, versioning.DependencyMeta{User: meta.User, Repo: meta.Repo, Tag: "1.0.0"})
	require.NoError(t, err)

	require.NoError(t, pcx.RecordDependencyResolution(meta, lockfile.DependencyResolution{Commit: reference.Hash().String(), Resolved: "1.0.0"}, false, ""))

	require.NoError(t, pcx.SaveLockfile())
	pinned := lf.Dependencies[lockfile.DependencyKey(meta)].Commit

	pcx.ForceUpdateLockfile()
	sentinel := errors.New("runtime unavailable")
	pcx.RuntimeProv = &fakeRuntimeProvisioner{binariesErr: sentinel}

	_, err = pcx.EnsureProject(context.Background(), DependencyUpdateRequest{Enabled: true, Force: true})
	require.ErrorIs(t, err, sentinel)

	pcx.RuntimeProv = &fakeRuntimeProvisioner{}
	_, err = pcx.EnsureProject(context.Background(), DependencyUpdateRequest{})
	require.NoError(t, err)

	final, err := lockfile.Load(pcx.Package.LocalPath)
	require.NoError(t, err)
	assert.Equal(t, pinned, final.Dependencies[lockfile.DependencyKey(meta)].Commit)
}

func TestLockOnlyRefreshesSharedRelationships(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	for _, name := range []string{"a", "b", "shared"} {
		seedEnsureProjectDependencyRepo(t, pcx.CacheDir, versioning.DependencyMeta{User: "fixture", Repo: name}, []string{"1.0.0"})
	}

	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/a:1.0.0", "fixture/b:1.0.0"}
	require.NoError(t, pcx.Package.WriteDefinition())

	require.NoError(t, pcx.InitLockfileResolver("test"))
	for _, name := range []string{"a", "b"} {
		meta := versioning.DependencyMeta{User: "fixture", Repo: name}
		repo, err := pcx.repositoryStore().Open(meta.CachePath(pcx.CacheDir))
		require.NoError(t, err)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(meta.CachePath(pcx.CacheDir), "pawn.json"), []byte(`{"dependencies":["fixture/shared:1.0.0"]}`), 0o644))
		_, err = wt.Add("pawn.json")
		require.NoError(t, err)
		head, err := wt.Commit("add shared dependency", &git.CommitOptions{Author: &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Unix(500, 0)}})
		require.NoError(t, err)
		require.NoError(t, repo.DeleteTag("1.0.0"))
		_, err = repo.CreateTag("1.0.0", head, nil)
		require.NoError(t, err)
	}

	require.NoError(t, pcx.UpdateProjectLockfile(context.Background(), DependencyUpdateRequest{}))
	lf := pcx.GetLockfile()
	assert.ElementsMatch(t, []string{"github.com/fixture/a", "github.com/fixture/b"}, lf.Dependencies["github.com/fixture/shared"].RequiredBy)

	meta := versioning.DependencyMeta{User: "fixture", Repo: "a"}
	repo, err := pcx.repositoryStore().Open(meta.CachePath(pcx.CacheDir))
	require.NoError(t, err)

	wt, err := repo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(meta.CachePath(pcx.CacheDir), "pawn.json"), []byte(`{}`), 0o644))

	_, err = wt.Add("pawn.json")
	require.NoError(t, err)

	head, err := wt.Commit("remove shared dependency", &git.CommitOptions{Author: &object.Signature{Name: "fixture", Email: "fixture@example.com", When: time.Unix(600, 0)}})
	require.NoError(t, err)

	require.NoError(t, repo.DeleteTag("1.0.0"))
	_, err = repo.CreateTag("1.0.0", head, nil)
	require.NoError(t, err)

	require.NoError(t, pcx.UpdateProjectLockfile(context.Background(), DependencyUpdateRequest{Enabled: true, Force: true}))
	assert.Equal(t, []string{"github.com/fixture/b"}, pcx.GetLockfile().Dependencies["github.com/fixture/shared"].RequiredBy)
	assert.NoDirExists(t, filepath.Join(pcx.Package.LocalPath, "dependencies", "a"))
}

func TestLockOnlyPreservesExistingPinnedCommit(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dep", Tag: "1.0.0"}
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})

	require.NoError(t, pcx.InitLockfileResolver("test"))

	require.NoError(t, pcx.Install(context.Background(), []versioning.DependencyString{"fixture/dep:1.0.0"}, false))

	pinned := pcx.GetLockfile().Dependencies[lockfile.DependencyKey(meta)].Commit
	lockPath := filepath.Join(pcx.Package.LocalPath, lockfile.Filename)
	before, err := os.ReadFile(lockPath)
	require.NoError(t, err)

	require.NoError(t, pcx.UpdateProjectLockfile(context.Background(), DependencyUpdateRequest{}))
	assert.Equal(t, pinned, pcx.GetLockfile().Dependencies[lockfile.DependencyKey(meta)].Commit)

	after, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestLockOnlyUpdatesDirectDependencyAlsoReachedTransitively(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	parent := versioning.DependencyMeta{User: "fixture", Repo: "parent", Tag: "1.0.0"}
	seedReliabilityRepo(t, parent.CachePath(pcx.CacheDir), `{"dependencies":["fixture/shared:latest"]}`)
	shared := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "shared", Tag: "latest"}
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, shared, []string{"1.0.0", "2.0.0"})
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/parent:1.0.0", "fixture/shared:latest"}
	require.NoError(t, pcx.Package.WriteDefinition())

	require.NoError(t, pcx.InitLockfileResolver("test"))

	repo, err := pcx.repositoryStore().Open(shared.CachePath(pcx.CacheDir))
	require.NoError(t, err)

	pinned := shared
	pinned.Tag = "1.0.0"
	old, err := resolveCachedDependencyLock(pinned, repo)
	require.NoError(t, err)

	require.NoError(t, pcx.RecordDependencyResolution(shared, old, false, ""))

	require.NoError(t, pcx.SaveLockfile())

	require.NoError(t, pcx.UpdateProjectLockfile(context.Background(), DependencyUpdateRequest{}))

	entry := pcx.GetLockfile().Dependencies[lockfile.DependencyKey(shared)]
	assert.Equal(t, "1.0.0", entry.Resolved)
	assert.False(t, entry.Transitive)
	assert.Equal(t, []string{lockfile.DependencyKey(parent)}, entry.RequiredBy)

	target, err := versioning.DependencyString("fixture/shared").Explode()
	require.NoError(t, err)

	require.NoError(t, pcx.UpdateProjectLockfile(context.Background(), DependencyUpdateRequest{Enabled: true, Target: "fixture/shared", TargetMeta: target}))

	entry = pcx.GetLockfile().Dependencies[lockfile.DependencyKey(shared)]
	assert.Equal(t, "2.0.0", entry.Resolved)
	assert.False(t, entry.Transitive)
	assert.Equal(t, []string{lockfile.DependencyKey(parent)}, entry.RequiredBy)
}

func TestLockOnlyRejectsConflictingDependencyDestinations(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	for _, owner := range []string{"owner-one", "owner-two"} {
		meta := versioning.DependencyMeta{User: owner, Repo: "shared", Tag: "1.0.0"}
		seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{}`)
	}

	pcx.Package.Dependencies = []versioning.DependencyString{"owner-one/shared:1.0.0", "owner-two/shared:1.0.0"}
	require.NoError(t, pcx.Package.WriteDefinition())

	require.NoError(t, pcx.InitLockfileResolver("test"))

	err := pcx.UpdateProjectLockfile(context.Background(), DependencyUpdateRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "share the installation directory")
	assert.Empty(t, pcx.GetLockfile().Dependencies)
	assert.NoFileExists(t, filepath.Join(pcx.Package.LocalPath, lockfile.Filename))
}

func TestUninstallBareSelectorRemovesPinnedDeclaration(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pcx := lifecycleContext(t, format)
			pcx.Package.Dependencies = []versioning.DependencyString{"fixture/dep:1.0.0"}
			require.NoError(t, pcx.Package.WriteDefinition())

			require.NoError(t, pcx.Uninstall([]versioning.DependencyString{"fixture/dep:2.0.0"}, false))
			assert.Equal(t, []versioning.DependencyString{"fixture/dep:1.0.0"}, pcx.Package.Dependencies)

			require.NoError(t, pcx.Uninstall([]versioning.DependencyString{"fixture/dep"}, false))
			assert.Empty(t, pcx.Package.Dependencies)

			persisted, err := pawnpackage.PackageFromDir(pcx.Package.LocalPath)
			require.NoError(t, err)
			assert.Empty(t, persisted.Dependencies)
		})
	}
}
