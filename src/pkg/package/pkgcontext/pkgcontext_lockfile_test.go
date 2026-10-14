package pkgcontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func TestPackageContextConstructorsResolveSelectedGraph(t *testing.T) {
	for _, name := range []string{"lockfile", "lockfile disabled", "plain", "load only"} {
		t.Run(name, func(t *testing.T) {
			options, parent, child, pinned := constructorGraphFixture(t)
			options.UseLockfile = name != "lockfile disabled"
			construct := NewPackageContextWithLockfile
			switch name {
			case "plain":
				construct = NewPackageContext
			case "load only":
				construct = LoadPackageContext
			}

			pcx, err := construct(options)
			require.NoError(t, err)
			assert.Equal(t, []versioning.DependencyString{"fixture/parent@release"}, pcx.Package.Dependencies)
			if name == "load only" {
				assert.Empty(t, pcx.AllDependencies)
				assert.False(t, pcx.HasLockfileResolver())
				return
			}
			if name != "lockfile" {
				assert.Equal(t, []versioning.DependencyMeta{parent}, pcx.AllDependencies)
				assert.Empty(t, pcx.AllPlugins)
				assert.Empty(t, pcx.AllIncludePaths)
				assert.False(t, pcx.HasLockfileResolver())
				return
			}

			assert.Equal(t, []versioning.DependencyMeta{parent, child}, pcx.AllDependencies)
			assert.Equal(t, []versioning.DependencyMeta{parent, child}, pcx.AllPlugins)
			assert.Equal(t, graphRevisionIncludePaths(pcx.Package.Vendor, "parent", "child"), pcx.AllIncludePaths)
			assert.Equal(t, pinned, pcx.LockedVersion(parent, false).Commit)
			assert.True(t, pcx.UseLockfile)
			assert.Equal(t, options.AppVersion, pcx.AppVersion)

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.ErrorIs(t, pcx.EnsureDependenciesCachedContext(ctx), context.Canceled)
			assert.Equal(t, []versioning.DependencyMeta{parent, child}, pcx.AllDependencies)

			require.NoError(t, pcx.EnsureDependenciesCachedContext(t.Context()))
			assert.Equal(t, []versioning.DependencyMeta{parent, child}, pcx.AllDependencies)
			assert.Equal(t, graphRevisionIncludePaths(pcx.Package.Vendor, "parent", "child"), pcx.AllIncludePaths)
		})
	}
}

func TestNewPackageContextWithLockfileHonorsChangedConstraint(t *testing.T) {
	options, parent, _, _ := constructorGraphFixture(t)
	options.UseLockfile = true
	lf, err := lockfile.Load(options.Dir)
	require.NoError(t, err)
	locked := lf.Dependencies[lockfile.DependencyKey(parent)]
	locked.Constraint = "@previous"
	lf.Dependencies[lockfile.DependencyKey(parent)] = locked
	require.NoError(t, lockfile.Save(options.Dir, lf))

	pcx, err := NewPackageContextWithLockfile(options)
	require.NoError(t, err)
	assert.True(t, pcx.HasLockfileResolver())
	assert.Equal(t, parent, pcx.LockedVersion(parent, false))
	assert.Equal(t, []versioning.DependencyMeta{parent}, pcx.AllDependencies)
	assert.Empty(t, pcx.AllPlugins)
	assert.Empty(t, pcx.AllIncludePaths)
}

func TestNewPackageContextWithLockfilePropagatesGraphErrors(t *testing.T) {
	for _, failure := range []error{os.ErrPermission, context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			options, _, _, _ := constructorGraphFixture(t)
			options.UseLockfile = true
			options.RepoStore = &fakeRepositoryStore{
				openFn: func(string) (*git.Repository, error) { return nil, failure },
				cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
					t.Fatal("existing local cache must not be cloned")
					return nil, nil
				},
			}

			pcx, err := NewPackageContextWithLockfile(options)
			require.ErrorIs(t, err, failure)
			assert.ErrorContains(t, err, "failed to ensure dependencies are cached")
			assert.Nil(t, pcx)
		})
	}
}

func TestNewPackageContextWithLockfileRejectsInvalidLockfileBeforeTraversal(t *testing.T) {
	options, _, _, _ := constructorGraphFixture(t)
	options.UseLockfile = true
	require.NoError(t, os.WriteFile(filepath.Join(options.Dir, lockfile.Filename), []byte("{"), 0o600))
	options.RepoStore = &fakeRepositoryStore{openFn: func(string) (*git.Repository, error) {
		t.Fatal("invalid lockfile must fail before dependency traversal")
		return nil, nil
	}}

	pcx, err := NewPackageContextWithLockfile(options)
	require.ErrorContains(t, err, "failed to initialize lockfile resolver")
	assert.Nil(t, pcx)
}

func constructorGraphFixture(t *testing.T) (NewPackageContextOptions, versioning.DependencyMeta, versioning.DependencyMeta, string) {
	t.Helper()

	options := NewPackageContextOptions{Dir: t.TempDir(), CacheDir: t.TempDir(), Parent: true, Platform: "linux", AppVersion: "test"}
	parent := graphRevisionMeta(t, "fixture/parent@release")
	child := graphRevisionMeta(t, "fixture/child:1.0.0")
	repo := seedReliabilityRepo(t, parent.CachePath(options.CacheDir), graphRevisionDefinition(t, "parent", "fixture/child:1.0.0"))
	pinned, err := repo.Head()
	require.NoError(t, err)
	childRepo := seedReliabilityRepo(t, child.CachePath(options.CacheDir), graphRevisionDefinition(t, "child"))
	childHead, err := childRepo.Head()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(parent.CachePath(options.CacheDir), "pawn.json"), []byte(`{"repo":"parent"}`), 0o644))
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	_, err = worktree.Add("pawn.json")
	require.NoError(t, err)
	head, err := worktree.Commit("advance cache", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(500, 0)}})
	require.NoError(t, err)
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("release"), head)))

	require.NoError(t, os.WriteFile(filepath.Join(options.Dir, "pawn.json"), []byte(`{"user":"fixture","repo":"root","dependencies":["fixture/parent@release"]}`), 0o644))
	lf := lockfile.New(options.AppVersion)
	lf.AddDependency(lockfile.DependencyKey(parent), lockfile.LockedDependency{Site: parent.Site, User: parent.User, Repo: parent.Repo, Constraint: "@release", Commit: pinned.Hash().String()})
	lf.AddDependency(lockfile.DependencyKey(child), lockfile.LockedDependency{Site: child.Site, User: child.User, Repo: child.Repo, Constraint: ":1.0.0", Commit: childHead.Hash().String(), Transitive: true})
	require.NoError(t, lockfile.Save(options.Dir, lf))

	return options, parent, child, pinned.Hash().String()
}
