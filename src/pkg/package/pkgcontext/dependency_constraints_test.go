package pkgcontext

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
)

func TestDependencyGraphUnpinnedReferencesUseDirectPins(t *testing.T) {
	for _, pin := range []string{"branch", "tag", "commit"} {
		for _, directFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/direct-first=%t", pin, directFirst), func(t *testing.T) {
				pcx := lifecycleContext(t, "json")
				meta := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "shared"}
				if pin == "branch" {
					meta.Branch = "master"
				}

				repo := seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{"dependencies":["fixture/selected-child:1.0.0"]}`)
				head, err := repo.Head()
				if err != nil {
					t.Fatal(err)
				}
				switch pin {
				case "tag":
					meta.Tag = "1.0.0"
				case "commit":
					meta.Commit = head.Hash().String()
				}

				for _, fixture := range []struct{ repo, definition string }{
					{"parent", `{"dependencies":["fixture/shared"]}`},
					{"selected-child", `{}`},
				} {
					seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: fixture.repo}).CachePath(pcx.CacheDir), fixture.definition)
				}

				declaration := versioning.DependencyString(formatPinnedDependency(meta))
				pcx.Package.Dependencies = []versioning.DependencyString{"fixture/parent:1.0.0", declaration}
				if directFirst {
					pcx.Package.Dependencies[0], pcx.Package.Dependencies[1] = pcx.Package.Dependencies[1], pcx.Package.Dependencies[0]
				}
				if err := pcx.Package.WriteDefinition(); err != nil {
					t.Fatal(err)
				}

				pcx.RepoStore = &fakeRepositoryStore{
					openFn: git.PlainOpen,
					cloneFn: func(_ string, _ bool, opts *git.CloneOptions) (*git.Repository, error) {
						return nil, fmt.Errorf("unexpected clone from %s", opts.URL)
					},
				}

				if err := pcx.EnsureDependenciesCachedContext(t.Context()); err != nil {
					t.Fatalf("EnsureDependenciesCachedContext(%v) error = %v, want nil", pcx.Package.Dependencies, err)
				}
				if got := len(pcx.AllDependencies); got != 3 {
					t.Errorf("EnsureDependenciesCachedContext(%v) resolved %d dependencies, want 3", pcx.Package.Dependencies, got)
				}

				for _, dependency := range pcx.AllDependencies {
					if dependency.Repo == meta.Repo && dependency != meta {
						t.Errorf("resolved shared dependency = %+v, want direct pin %+v", dependency, meta)
					}
				}

				if err := pcx.InitLockfileResolver("test"); err != nil {
					t.Fatal(err)
				}

				if err := pcx.UpdateProjectLockfile(t.Context(), DependencyUpdateRequest{}); err != nil {
					t.Fatalf("UpdateProjectLockfile(%v) error = %v, want nil", pcx.Package.Dependencies, err)
				}

				entry, ok := pcx.GetLockfile().GetDependency(lockfile.DependencyKey(meta))
				if !ok || entry.Transitive || entry.Commit != head.Hash().String() {
					t.Errorf("GetDependency(%s) = %+v, %t, want direct pin at %s", meta, entry, ok, head.Hash())
				}

				if len(entry.RequiredBy) != 1 || entry.RequiredBy[0] != "github.com/fixture/parent" {
					t.Errorf("GetDependency(%s).RequiredBy = %v, want [github.com/fixture/parent]", meta, entry.RequiredBy)
				}
			})
		}
	}
}

func TestDependencyGraphStillRejectsConflictingExplicitPins(t *testing.T) {
	pcx := lifecycleContext(t, "json")
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/parent:1.0.0", "fixture/shared@master"}
	if err := pcx.Package.WriteDefinition(); err != nil {
		t.Fatal(err)
	}

	seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: "parent"}).CachePath(pcx.CacheDir), `{"dependencies":["fixture/shared@other"]}`)
	pcx.RepoStore = &fakeRepositoryStore{
		openFn: git.PlainOpen,
		cloneFn: func(_ string, _ bool, opts *git.CloneOptions) (*git.Repository, error) {
			return nil, fmt.Errorf("unexpected clone from %s", opts.URL)
		},
	}
	// Seed both branches so a missing repository cannot hide a missed conflict.
	for _, branch := range []string{"master", "other"} {
		seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: "shared", Branch: branch}).CachePath(pcx.CacheDir), `{}`)
	}

	if err := pcx.EnsureDependenciesCachedContext(t.Context()); err == nil {
		t.Errorf("EnsureDependenciesCachedContext(%v) error = nil, want conflicting explicit pins rejected", pcx.Package.Dependencies)
	}

	if err := pcx.InitLockfileResolver("test"); err != nil {
		t.Fatal(err)
	}

	if err := pcx.UpdateProjectLockfile(t.Context(), DependencyUpdateRequest{}); err == nil {
		t.Errorf("UpdateProjectLockfile(%v) error = nil, want conflicting explicit pins rejected", pcx.Package.Dependencies)
	}
}

func TestEnsureDependenciesPreloadedUnpinnedReferenceUsesDirectPin(t *testing.T) {
	pcx, _ := reliabilityPackageContext(t)
	meta := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "shared", Branch: "master"}
	pcx.Package.Dependencies = []versioning.DependencyString{"fixture/shared@master"}
	unpinned := meta
	unpinned.Branch = ""
	pcx.AllDependencies = []versioning.DependencyMeta{unpinned, meta}
	seedReliabilityRepo(t, meta.CachePath(pcx.CacheDir), `{}`)
	pcx.RepoStore = &fakeRepositoryStore{
		openFn: git.PlainOpen,
		cloneFn: func(path string, bare bool, opts *git.CloneOptions) (*git.Repository, error) {
			if !filepath.IsAbs(opts.URL) {
				return nil, fmt.Errorf("unexpected remote clone from %s", opts.URL)
			}

			return git.PlainClone(path, bare, opts)
		},
	}

	if err := pcx.EnsureDependencies(context.Background(), false); err != nil {
		t.Fatalf("EnsureDependencies(%v) error = %v, want nil", pcx.AllDependencies, err)
	}
}
