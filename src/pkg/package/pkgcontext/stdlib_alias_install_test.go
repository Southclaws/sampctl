package pkgcontext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestInstallLegacyStdlibReferencesPreservesDirectPin(t *testing.T) {
	for _, test := range []struct{ format, repo, legacy string }{
		{"json", "samp-stdlib", "Southclaws/samp-stdlib"},
		{"yaml", "samp-stdlib", "Southclaws/samp-stdlib"},
		{"json", "pawn-stdlib", "sampctl/pawn-stdlib"},
		{"yaml", "pawn-stdlib", "sampctl/pawn-stdlib"},
	} {
		t.Run(test.format+"/"+test.repo, func(t *testing.T) {
			pcx := lifecycleContext(t, test.format)
			pcx.Package.Preset = "openmp"
			meta := versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: test.repo, Branch: "open.mp"}
			pcx.Package.Dependencies = []versioning.DependencyString{versioning.DependencyString("pawn-lang/" + test.repo + "@open.mp")}
			if err := pcx.Package.WriteDefinition(); err != nil {
				t.Fatal(err)
			}

			pinned, err := seedPkgContextRepo(pcx.CacheDir, meta, pawnpackage.Package{User: meta.User, Repo: meta.Repo}, nil, meta.Branch,
				map[string]string{"a_samp.inc": "// locked open.mp includes\n"})
			if err != nil {
				t.Fatal(err)
			}

			if err := pcx.InitLockfileResolver("test"); err != nil {
				t.Fatal(err)
			}

			if err := pcx.RecordDependencyResolution(meta, lockfile.DependencyResolution{Commit: pinned, Resolved: meta.Branch}, false, ""); err != nil {
				t.Fatal(err)
			}

			if err := pcx.SaveLockfile(); err != nil {
				t.Fatal(err)
			}

			// A newer branch tip must not replace the project's existing lock.
			repo, err := git.PlainOpen(meta.CachePath(pcx.CacheDir))
			if err != nil {
				t.Fatal(err)
			}

			worktree, err := repo.Worktree()
			if err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(meta.CachePath(pcx.CacheDir), "a_samp.inc"), []byte("// newer branch includes\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := worktree.Add("a_samp.inc"); err != nil {
				t.Fatal(err)
			}

			newer, err := worktree.Commit("newer", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com"}})
			if err != nil {
				t.Fatal(err)
			}

			if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(meta.Branch), newer)); err != nil {
				t.Fatal(err)
			}

			for _, fixture := range []struct{ repo, definition string }{
				{"progress", fmt.Sprintf(`{"dependencies":[%q,"fixture/indirect:1.0.0","sampctl/%s"]}`, test.legacy, test.repo)},
				{"indirect", fmt.Sprintf(`{"dependencies":["sampctl/%s","sampctl/%s"]}`, test.repo, test.repo)},
			} {
				seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: fixture.repo}).CachePath(pcx.CacheDir), fixture.definition)
			}
			pcx.RepoStore = &fakeRepositoryStore{
				openFn: git.PlainOpen,
				cloneFn: func(path string, bare bool, opts *git.CloneOptions) (*git.Repository, error) {
					if !filepath.IsAbs(opts.URL) {
						return nil, fmt.Errorf("unexpected remote clone from %s", opts.URL)
					}

					return git.PlainClone(path, bare, opts)
				},
			}

			if err := pcx.Install(t.Context(), []versioning.DependencyString{"fixture/progress:1.0.0"}, false); err != nil {
				t.Fatalf("Install(progress with legacy stdlib references) = %v, want success", err)
			}

			if len(pcx.AllDependencies) != 3 {
				t.Errorf("Install(progress).AllDependencies = %v, want one stdlib plus progress and indirect", pcx.AllDependencies)
			}

			persisted, err := lockfile.Load(pcx.Package.LocalPath)
			if err != nil || persisted == nil {
				t.Fatalf("Load(pawn.lock) = %v, %v, want a saved lockfile", persisted, err)
			}
			stdlib, ok := persisted.GetDependency(lockfile.DependencyKey(meta))
			if !ok || stdlib.Commit != pinned || stdlib.Constraint != "@open.mp" || stdlib.Transitive {
				t.Errorf("Install(progress) stdlib lock = %+v, %t, want direct @open.mp pin at %s", stdlib, ok, pinned)
			}
			if want := []string{"github.com/fixture/indirect", "github.com/fixture/progress"}; !slices.Equal(stdlib.RequiredBy, want) {
				t.Errorf("Install(progress) stdlib parents = %v, want %v", stdlib.RequiredBy, want)
			}

			if len(persisted.Dependencies) != 3 {
				t.Errorf("Install(progress) locked dependencies = %v, want one canonical stdlib entry and two consumers", persisted.Dependencies)
			}

			installed, err := os.ReadFile(filepath.Join(pcx.Package.Vendor, meta.Repo, "a_samp.inc"))
			if err != nil || string(installed) != "// locked open.mp includes\n" {
				t.Errorf("Install(progress) installed stdlib = %q, %v, want the locked open.mp includes", installed, err)
			}

			for _, owner := range []string{"Southclaws", "sampctl"} {
				if _, err := os.Lstat((versioning.DependencyMeta{User: owner, Repo: meta.Repo}).CachePath(pcx.CacheDir)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Install(progress) created a legacy %s stdlib cache, stat error = %v, want os.ErrNotExist", owner, err)
				}
			}
			assertNoDependencyStages(t, pcx.Package.Vendor)
		})
	}
}

func TestStdlibAliasGraphRejectsDifferentRepositoryOrExplicitPin(t *testing.T) {
	for _, test := range []struct {
		repo        string
		declaration versioning.DependencyString
	}{
		{"samp-stdlib", "Southclaws/samp-stdlib@other"},
		{"samp-stdlib", "unrelated/samp-stdlib"},
		{"pawn-stdlib", "sampctl/pawn-stdlib@other"},
		{"pawn-stdlib", "unrelated/pawn-stdlib"},
	} {
		t.Run(string(test.declaration), func(t *testing.T) {
			declaration := test.declaration
			pcx := lifecycleContext(t, "json")
			meta := versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: test.repo, Branch: "open.mp"}
			pcx.Package.Dependencies = []versioning.DependencyString{versioning.DependencyString("pawn-lang/" + test.repo + "@open.mp"), "fixture/progress:1.0.0"}
			if err := pcx.Package.WriteDefinition(); err != nil {
				t.Fatal(err)
			}

			if _, err := seedPkgContextRepo(pcx.CacheDir, meta, pawnpackage.Package{User: meta.User, Repo: meta.Repo}, nil, meta.Branch, nil); err != nil {
				t.Fatal(err)
			}
			seedReliabilityRepo(t, (versioning.DependencyMeta{User: "fixture", Repo: "progress"}).CachePath(pcx.CacheDir), fmt.Sprintf(`{"dependencies":[%q]}`, declaration))
			pcx.RepoStore = &fakeRepositoryStore{
				openFn: git.PlainOpen,
				cloneFn: func(_ string, _ bool, opts *git.CloneOptions) (*git.Repository, error) {
					return nil, fmt.Errorf("unexpected clone from %s", opts.URL)
				},
			}

			err := pcx.EnsureDependenciesCachedContext(t.Context())
			var conflict *DependencyDestinationConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("EnsureDependenciesCachedContext(%s) = %v, want a destination conflict", declaration, err)
			}
			if conflict.First != meta || conflict.Directory != meta.Repo || conflict.Second == meta {
				t.Errorf("EnsureDependenciesCachedContext(%s) conflict = %+v, want the distinct repository or explicit pin preserved", declaration, conflict)
			}

			if _, err := os.Lstat(pcx.Package.Vendor); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("EnsureDependenciesCachedContext(%s) created installed dependencies, stat = %v, want os.ErrNotExist", declaration, err)
			}
		})
	}
}
