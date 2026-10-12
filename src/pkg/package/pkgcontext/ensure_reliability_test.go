package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestEnsureDependenciesReportsAllFailures(t *testing.T) {
	t.Parallel()
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			project := t.TempDir()
			deps := []versioning.DependencyMeta{
				{Scheme: "includes", Local: "missing-a"},
				{Scheme: "includes", Local: "missing-b"},
			}
			pcx := &PackageContext{
				Package:              pawnpackage.Package{LocalPath: project},
				PackageResolvedState: PackageResolvedState{AllDependencies: deps[:count]},
			}
			err := pcx.EnsureDependencies(context.Background(), false)
			require.Error(t, err)
			for _, dep := range deps[:count] {
				assert.Contains(t, err.Error(), dep.Local)
			}
		})
	}
}

func TestEnsureDependenciesValidatesDeclaredLocalPaths(t *testing.T) {
	t.Parallel()
	pcx := &PackageContext{Package: pawnpackage.Package{
		LocalPath: t.TempDir(), Dependencies: []versioning.DependencyString{"includes://local/missing"},
	}}
	err := pcx.EnsureDependencies(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "includes://local/missing")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestEnsureDependenciesRejectsInstallationPathCollision(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	other := meta
	other.User = "other"
	pcx.AllDependencies = []versioning.DependencyMeta{meta, other}
	pcx.RepoStore = &fakeRepositoryStore{
		openFn: func(string) (*git.Repository, error) {
			t.Fatal("colliding dependencies should be rejected before installation")
			return nil, nil
		},
	}
	err := pcx.EnsureDependencies(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), meta.String())
	assert.Contains(t, err.Error(), other.String())
}

func TestDependencyGraphRejectsTransitiveInstallationPathCollision(t *testing.T) {
	t.Parallel()
	pcx, _ := reliabilityPackageContext(t)
	pcx.Package.Parent = true
	pcx.Package.Repo = "root"
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/parent-a:1.0.0", "fixture/parent-b:1.0.0"}
	for _, fixture := range []struct{ user, repo, definition string }{
		{"fixture", "parent-a", `{"dependencies":["owner-one/shared:1.0.0"]}`},
		{"fixture", "parent-b", `{"dependencies":["owner-two/shared:1.0.0"]}`},
		{"owner-one", "shared", `{}`},
	} {
		meta := versioning.DependencyMeta{User: fixture.user, Repo: fixture.repo}
		seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), fixture.definition)
	}
	pcx.RepoStore = &fakeRepositoryStore{
		openFn: GitRepositoryStore{}.Open,
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
			t.Fatal("collision must be detected before retrieving another owner's repository")
			return nil, nil
		},
	}
	err := pcx.EnsureDependenciesCachedContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "owner-one/shared")
	assert.Contains(t, err.Error(), "owner-two/shared")
}

func TestEnsureDependenciesStopsOnCancellation(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	other := meta
	other.Repo = "next-dependency"
	pcx.AllDependencies = []versioning.DependencyMeta{meta, other}
	store := &fakeRepositoryStore{
		openFn:  func(string) (*git.Repository, error) { return nil, git.ErrRepositoryNotExists },
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) { return nil, context.Canceled },
	}
	pcx.RepoStore = store
	require.ErrorIs(t, pcx.EnsureDependencies(context.Background(), false), context.Canceled)
	assert.Equal(t, 1, store.cloneCalls)
	assert.NoDirExists(t, filepath.Join(pcx.Package.Vendor, other.Repo))
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestEnsurePackagePreservesSymlinkTargets(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"vendor", "dependency", "Git metadata"} {
		t.Run(boundary, func(t *testing.T) {
			pcx, meta := reliabilityPackageContext(t)
			external := t.TempDir()
			writeReliabilityContent(t, external, "external")
			link := pcx.Package.Vendor
			if boundary != "vendor" {
				require.NoError(t, os.MkdirAll(pcx.Package.Vendor, 0o755))
				link = filepath.Join(link, meta.Repo)
				if boundary == "Git metadata" {
					require.NoError(t, os.MkdirAll(link, 0o755))
					link = filepath.Join(link, ".git")
				}
			}
			if err := os.Symlink(external, link); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlinks unavailable: %v", err)
				}
				require.NoError(t, err)
			}
			require.Error(t, pcx.ensurePackage(context.Background(), meta, false))
			contents, err := os.ReadFile(filepath.Join(external, "keep.inc"))
			require.NoError(t, err)
			assert.Equal(t, "external", string(contents))
			info, err := os.Lstat(link)
			require.NoError(t, err)
			assert.NotZero(t, info.Mode()&os.ModeSymlink)
		})
	}
}

func TestEnsureDependenciesPreservesExternalIncludePath(t *testing.T) {
	t.Parallel()
	shared := t.TempDir()
	project := filepath.Join(shared, "project")
	external := filepath.Join(shared, "external")
	require.NoError(t, os.MkdirAll(project, 0o755))
	writeReliabilityContent(t, external, "external")
	pcx := &PackageContext{Package: pawnpackage.Package{
		LocalPath: project, Dependencies: []versioning.DependencyString{"includes://local/../external"},
	}}
	require.NoError(t, pcx.EnsureDependencies(context.Background(), false))
	contents, err := os.ReadFile(filepath.Join(external, "keep.inc"))
	require.NoError(t, err)
	assert.Equal(t, "external", string(contents))
}

func TestDependencyInstallStateRestoresPreviousInstallation(t *testing.T) {
	t.Parallel()
	state := newDependencyInstallState()
	vendor := t.TempDir()
	stage, err := state.stage(vendor)
	require.NoError(t, err)
	old := filepath.Join(vendor, "dependency")
	newPath := filepath.Join(stage, "dependency")
	writeReliabilityContent(t, old, "previous")
	writeReliabilityContent(t, newPath, "replacement")
	require.NoError(t, state.promote(newPath, old))
	require.NoError(t, state.rollback())
	contents, err := os.ReadFile(filepath.Join(old, "keep.inc"))
	require.NoError(t, err)
	assert.Equal(t, "previous", string(contents))
	assertNoDependencyStages(t, vendor)
}

func TestDependencyInstallStateRejectsSymlinkedResourceAncestor(t *testing.T) {
	t.Parallel()
	state := newDependencyInstallState()
	vendor := t.TempDir()
	stage, err := state.stage(vendor)
	require.NoError(t, err)
	external := t.TempDir()
	writeReliabilityContent(t, external, "external")
	if err := os.Symlink(external, filepath.Join(vendor, ".resources")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		require.NoError(t, err)
	}
	source := filepath.Join(stage, ".resources", "dependency")
	writeReliabilityContent(t, source, "replacement")
	target := filepath.Join(vendor, ".resources", "new-directory", "dependency")
	require.Error(t, state.promote(source, target))
	assert.NoDirExists(t, filepath.Join(external, "new-directory"))
	contents, err := os.ReadFile(filepath.Join(external, "keep.inc"))
	require.NoError(t, err)
	assert.Equal(t, "external", string(contents))
	require.NoError(t, state.rollback())
	assertNoDependencyStages(t, vendor)
}

func TestDependencyInstallStateReportsPromotionRollbackAndCleanupFailures(t *testing.T) {
	t.Parallel()
	for _, boundary := range []string{"promotion", "rollback", "cleanup"} {
		t.Run(boundary, func(t *testing.T) {
			state := newDependencyInstallState()
			vendor := t.TempDir()
			stage, err := state.stage(vendor)
			require.NoError(t, err)
			old := filepath.Join(vendor, "dependency")
			newPath := filepath.Join(stage, "dependency")
			writeReliabilityContent(t, old, "previous")
			writeReliabilityContent(t, newPath, "replacement")
			failure := errors.New("injected filesystem failure")
			if boundary == "promotion" {
				state.rename = func(from, to string) error {
					if from == newPath {
						return failure
					}
					return os.Rename(from, to)
				}
				require.ErrorIs(t, state.promote(newPath, old), failure)
				require.NoError(t, state.rollback())
				contents, err := os.ReadFile(filepath.Join(old, "keep.inc"))
				require.NoError(t, err)
				assert.Equal(t, "previous", string(contents))
				return
			}
			require.NoError(t, state.promote(newPath, old))
			if boundary == "rollback" {
				state.rename = func(string, string) error { return failure }
				require.ErrorIs(t, state.rollback(), failure)
				contents, err := os.ReadFile(filepath.Join(state.installations[0].backup, "keep.inc"))
				require.NoError(t, err)
				assert.Equal(t, "previous", string(contents), "retain backup after unsuccessful rollback")
				return
			}
			state.removeAll = func(string) error { return failure }
			require.ErrorIs(t, state.commit(), failure)
			assert.FileExists(t, filepath.Join(old, "keep.inc"))
			assert.DirExists(t, stage)
		})
	}
}

func TestUpdateRepoStateDoesNotRetryPermanentFailure(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	path := filepath.Join(pcx.Package.Vendor, meta.Repo)
	repo := seedReliabilityRepo(t, path, `{}`)
	health := &fakeRepositoryHealth{
		validateFn: func(string) (bool, error) { return true, nil },
		repairFn:   func(string) error { t.Fatal("permanent reference failure should not trigger repair"); return nil },
	}
	pcx.RepoHealth = health
	meta.Tag = "does-not-exist"
	require.Error(t, pcx.updateRepoStateWithRecoveryContext(context.Background(), repo, meta, path, false))
	assert.Zero(t, health.repairCalls)
}

func writeReliabilityContent(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "keep.inc"), []byte(contents), 0o644))
}

func TestEnsurePackagePreservesPreviousInstallationOnCloneFailure(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	previous := filepath.Join(pcx.Package.Vendor, meta.Repo)
	require.NoError(t, os.MkdirAll(filepath.Join(previous, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(previous, "keep.inc"), []byte("previous"), 0o644))
	store := &fakeRepositoryStore{
		openFn:  func(string) (*git.Repository, error) { return nil, git.ErrRepositoryNotExists },
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) { return nil, os.ErrPermission },
	}
	pcx.RepoStore = store
	err := pcx.ensurePackage(context.Background(), meta, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)
	contents, readErr := os.ReadFile(filepath.Join(previous, "keep.inc"))
	require.NoError(t, readErr)
	assert.Equal(t, "previous", string(contents))
	assert.Equal(t, 1, store.cloneCalls)
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestEnsurePackageCancellationPreservesInstallation(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	ctx, cancel := context.WithCancel(context.Background())
	pcx.RepoHealth = &fakeRepositoryHealth{
		validateFn: func(string) (bool, error) { cancel(); return false, nil },
		repairFn:   func(string) error { return nil },
	}
	previous := filepath.Join(pcx.Package.Vendor, meta.Repo)
	require.NoError(t, os.MkdirAll(filepath.Join(previous, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(previous, "keep.inc"), []byte("previous"), 0o644))
	store := &fakeRepositoryStore{
		openFn: func(string) (*git.Repository, error) { return nil, git.ErrRepositoryNotExists },
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
			t.Fatal("clone after cancellation")
			return nil, nil
		},
	}
	pcx.RepoStore = store
	err := pcx.ensurePackage(ctx, meta, false)
	require.ErrorIs(t, err, context.Canceled)
	assert.FileExists(t, filepath.Join(previous, "keep.inc"))
	assert.Zero(t, store.cloneCalls)
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestEnsurePackageDoesNotRecordFailedResources(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	cache := meta.CachePath(pcx.CacheDir)
	seedReliabilityRepo(t, cache, `{"resources":[{"name":"missing.zip","platform":"linux","archive":true,"includes":["include"]}]}`)
	resolver := &fakeDependencyLock{lockfile: lockfile.New("test")}
	pcx.PackageLockfileState.SetLockfileResolver(resolver)
	err := pcx.ensurePackage(context.Background(), meta, false)
	require.Error(t, err)
	assert.Empty(t, resolver.lastResolution.Commit)
	assert.NoDirExists(t, filepath.Join(pcx.Package.Vendor, meta.Repo))
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestResourcePackageDefinitionPropagatesCancellation(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	meta.Tag = ""
	pcx.RemotePackages = &fakeRemotePackageFetcher{err: context.Canceled}
	_, err := pcx.resourcePackageDefinition(context.Background(), meta)
	require.ErrorIs(t, err, context.Canceled)
}

func TestEnsurePackageSuccessfulAndRepeatedInstallation(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{}`)
	require.NoError(t, pcx.ensurePackage(context.Background(), meta, false))
	installed := filepath.Join(pcx.Package.Vendor, meta.Repo)
	before, err := os.Stat(installed)
	require.NoError(t, err)
	require.NoError(t, pcx.ensurePackage(context.Background(), meta, false))
	after, err := os.Stat(installed)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "an unchanged installation should retain its directory")
	assert.FileExists(t, filepath.Join(installed, "pawn.json"))
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestEnsurePackageRecoversEmptyExistingDirectory(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{}`)
	installed := filepath.Join(pcx.Package.Vendor, meta.Repo)
	require.NoError(t, os.MkdirAll(installed, 0o755))
	require.NoError(t, pcx.ensurePackage(context.Background(), meta, false))
	assert.FileExists(t, filepath.Join(installed, "pawn.json"))
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestEnsurePackagePreservesNonemptyDirectoryWithoutGitMetadata(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{}`)
	installed := filepath.Join(pcx.Package.Vendor, meta.Repo)
	writeReliabilityContent(t, installed, "user data")
	require.Error(t, pcx.ensurePackage(context.Background(), meta, false))
	contents, err := os.ReadFile(filepath.Join(installed, "keep.inc"))
	require.NoError(t, err)
	assert.Equal(t, "user data", string(contents))
	assertNoDependencyStages(t, pcx.Package.Vendor)
}

func TestEnsurePackageRespectsPinsAndExplicitUpdate(t *testing.T) {
	t.Parallel()
	pcx, meta := reliabilityPackageContext(t)
	seedEnsureProjectDependencyRepo(t, pcx.CacheDir, meta, []string{"1.0.0", "2.0.0"})
	resolver, err := lockfile.NewResolver(pcx.Package.LocalPath, "test", true)
	require.NoError(t, err)
	pcx.SetLockfileResolver(resolver)
	require.NoError(t, pcx.ensurePackage(context.Background(), meta, false))
	first, ok := pcx.GetLockfile().GetDependency(lockfile.DependencyKey(meta))
	require.True(t, ok)
	require.NoError(t, pcx.ensurePackage(context.Background(), meta, false))
	pinned, _ := pcx.GetLockfile().GetDependency(lockfile.DependencyKey(meta))
	assert.Equal(t, first.Commit, pinned.Commit)
	meta.Tag = "2.0.0"
	require.NoError(t, pcx.ensurePackage(context.Background(), meta, true))
	updated, _ := pcx.GetLockfile().GetDependency(lockfile.DependencyKey(meta))
	assert.NotEqual(t, first.Commit, updated.Commit)
	assert.Equal(t, "2.0.0", updated.Resolved)
}

func TestEnsurePackageRejectsUnsafeManagedPaths(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"..", "../external", "", ".resources", `..\external`} {
		t.Run(name, func(t *testing.T) {
			pcx, meta := reliabilityPackageContext(t)
			meta.Repo = name
			pcx.RepoStore = &fakeRepositoryStore{
				openFn: func(string) (*git.Repository, error) { t.Fatal("unsafe path reached Git"); return nil, nil },
			}
			require.Error(t, pcx.ensurePackage(context.Background(), meta, false))
		})
	}
}

func reliabilityPackageContext(t *testing.T) (*PackageContext, versioning.DependencyMeta) {
	t.Helper()
	project := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "dependency", Tag: "1.0.0"}
	pcx := &PackageContext{
		Package:         pawnpackage.Package{LocalPath: project, Vendor: filepath.Join(project, "dependencies")},
		PackageServices: PackageServices{CacheDir: t.TempDir(), Platform: "linux"},
	}
	return pcx, meta
}

func seedReliabilityRepo(t *testing.T, path, definition string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(path, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(path, "pawn.json"), []byte(definition), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("pawn.json")
	require.NoError(t, err)
	hash, err := wt.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com"}})
	require.NoError(t, err)
	_, err = repo.CreateTag("1.0.0", hash, nil)
	require.NoError(t, err)
	return repo
}

func assertNoDependencyStages(t *testing.T, vendor string) {
	t.Helper()
	stages, err := filepath.Glob(filepath.Join(vendor, ".sampctl-*"))
	require.NoError(t, err)
	assert.Empty(t, stages)
}

func TestLatestTagCancellationDoesNotReadCacheOrRetry(t *testing.T) {
	pcx := &PackageContext{PackageServices: PackageServices{CacheDir: t.TempDir()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := pcx.resolveLatestTag(ctx, versioning.DependencyMeta{User: "fixture", Repo: "dep"}, true)
	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(pcx.CacheDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
