package rook

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-github/github"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
	runtimepkg "github.com/Southclaws/sampctl/src/pkg/runtime"
	runtimecfg "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

func TestOpenMPStarterDependenciesResolveAndInstallOffline(t *testing.T) {
	projectDir, cacheDir := t.TempDir(), t.TempDir()
	dependencies := stdDependenciesForPreset("openmp")
	for _, declaration := range dependencies {
		meta, err := declaration.Explode()
		if err != nil {
			t.Fatalf("Explode(%q) error = %v", declaration, err)
		}
		pkg := pawnpackage.Package{Site: meta.Site, User: meta.User, Repo: meta.Repo}
		switch meta.Repo {
		case "samp-stdlib", "pawn-stdlib":
			pkg.Dependencies = []versioning.DependencyString{"openmultiplayer/omp-stdlib"}
		}
		if err := seedCachedPackageRepo(cacheDir, meta, pkg, nil, map[string]string{
			"selected.inc": meta.Branch,
		}); err != nil {
			t.Fatalf("seedCachedPackageRepo(%q) error = %v", declaration, err)
		}
		repo, err := git.PlainOpen(meta.CachePath(cacheDir))
		if err != nil {
			t.Fatalf("PlainOpen(%q) error = %v", declaration, err)
		}
		head, err := repo.Head()
		if err != nil {
			t.Fatalf("Head(%q) error = %v", declaration, err)
		}
		branch := plumbing.NewBranchReferenceName(meta.Branch)
		if err := repo.Storer.SetReference(plumbing.NewHashReference(branch, head.Hash())); err != nil {
			t.Fatalf("SetReference(%q) error = %v", declaration, err)
		}
		if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, branch)); err != nil {
			t.Fatalf("SetReference(HEAD, %q) error = %v", declaration, err)
		}
	}

	pkg := pawnpackage.Package{
		Parent: true, LocalPath: projectDir, Format: "json", User: "local-user", Repo: "starter",
		Preset: "openmp", Entry: defaultGamemodeEntry, Output: "gamemodes/main.amx",
		Dependencies: dependencies,
	}
	if err := pkg.WriteDefinition(); err != nil {
		t.Fatalf("WriteDefinition(open.mp starter) error = %v", err)
	}

	if err := writeGeneratedEntryFile(filepath.Join(projectDir, pkg.Entry), generatedGamemodeContents("openmp")); err != nil {
		t.Fatalf("writeGeneratedEntryFile(open.mp starter) error = %v", err)
	}

	client := github.NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected network request: %s", req.URL)
	})})
	provisioner := &starterRuntimeProvisioner{}
	pcx, err := pkgcontext.NewPackageContext(pkgcontext.NewPackageContextOptions{
		GitHub: client, Parent: true, Dir: projectDir, Platform: "linux", CacheDir: cacheDir, Init: true,
		RepoStore: starterLocalRepositoryStore{}, RuntimeProv: provisioner,
	})
	if err != nil {
		t.Fatalf("NewPackageContext(open.mp starter) error = %v, want nil", err)
	}

	if err := pcx.InitLockfileResolver("test"); err != nil {
		t.Fatalf("InitLockfileResolver(open.mp starter) error = %v", err)
	}

	if _, err := pcx.EnsureProject(context.Background(), pkgcontext.DependencyUpdateRequest{Enabled: true, Force: true}); err != nil {
		t.Fatalf("EnsureProject(open.mp starter, force update) error = %v, want nil", err)
	}

	originalLock, err := lockfile.Load(projectDir)
	if err != nil || originalLock == nil {
		t.Fatalf("Load(initial open.mp starter lockfile) = %v, %v, want lockfile and nil", originalLock, err)
	}

	for _, declaration := range dependencies {
		advanceCachedStarterBranch(t, cacheDir, declaration)
	}

	if _, err := pcx.EnsureProject(context.Background(), pkgcontext.DependencyUpdateRequest{}); err != nil {
		t.Fatalf("EnsureProject(open.mp starter, locked) error = %v, want nil", err)
	}

	if got := len(pcx.AllDependencies); got != len(dependencies) {
		t.Errorf("EnsureProject(open.mp starter, locked) resolved %d dependencies, want %d", got, len(dependencies))
	}

	if got := provisioner.binariesCalls; got != 2 {
		t.Errorf("EnsureProject(open.mp starter) ensured runtime binaries %d times, want 2", got)
	}

	lf, err := lockfile.Load(projectDir)
	if err != nil || lf == nil {
		t.Fatalf("Load(open.mp starter lockfile) = %v, %v, want lockfile and nil", lf, err)
	}
	if got := lf.DependencyCount(); got != len(dependencies) {
		t.Errorf("Load(open.mp starter lockfile) dependency count = %d, want %d", got, len(dependencies))
	}

	for _, declaration := range dependencies {
		meta, err := declaration.Explode()
		if err != nil {
			t.Fatalf("Explode(%q) error = %v", declaration, err)
		}
		path := filepath.Join(projectDir, "dependencies", meta.Repo, "selected.inc")
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("ReadFile(%q) error = %v, want nil", path, err)
		} else if got := string(contents); got != meta.Branch {
			t.Errorf("ReadFile(%q) = %q, want direct dependency branch %q", path, got, meta.Branch)
		}
		entry, ok := lf.GetDependency(lockfile.DependencyKey(meta))
		if !ok || entry.Transitive || entry.Constraint != "@"+meta.Branch {
			t.Errorf("GetDependency(%q) = %+v, %t, want direct entry with constraint %q", declaration, entry, ok, "@"+meta.Branch)
		}
		originalEntry, originallyLocked := originalLock.GetDependency(lockfile.DependencyKey(meta))
		if !originallyLocked || entry.Commit != originalEntry.Commit {
			t.Errorf("GetDependency(%q) commit = %q, want original lock commit %q", declaration, entry.Commit, originalEntry.Commit)
		}
	}
}

func advanceCachedStarterBranch(t *testing.T, cacheDir string, declaration versioning.DependencyString) {
	t.Helper()
	meta, err := declaration.Explode()
	if err != nil {
		t.Fatalf("Explode(%q) error = %v", declaration, err)
	}

	dir := meta.CachePath(cacheDir)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen(%q) error = %v", declaration, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree(%q) error = %v", declaration, err)
	}

	for name, contents := range map[string]string{"selected.inc": "new " + meta.Branch, "pawn.json": "{invalid"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile(%q, %q) error = %v", declaration, name, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("Add(%q, %q) error = %v", declaration, name, err)
		}
	}

	if _, err := wt.Commit("advance cached branch", &git.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(200, 0)},
	}); err != nil {
		t.Fatalf("Commit(%q) error = %v", declaration, err)
	}
}

type starterLocalRepositoryStore struct{}

func (starterLocalRepositoryStore) Open(path string) (*git.Repository, error) {
	return git.PlainOpen(path)
}

func (starterLocalRepositoryStore) Clone(path string, bare bool, opts *git.CloneOptions) (*git.Repository, error) {
	if !filepath.IsAbs(opts.URL) {
		return nil, fmt.Errorf("unexpected remote clone: %s", opts.URL)
	}
	return git.PlainClone(path, bare, opts)
}

type starterRuntimeProvisioner struct {
	binariesCalls int
}

func (*starterRuntimeProvisioner) EnsurePackageLayout(dir string, openMP bool) error {
	return fs.EnsurePackageLayout(dir, openMP)
}

func (p *starterRuntimeProvisioner) EnsureBinaries(_ context.Context, _ string, cfg runtimecfg.Runtime) (*runtimepkg.RuntimeManifestInfo, error) {
	p.binariesCalls++
	return &runtimepkg.RuntimeManifestInfo{Version: cfg.Version, Platform: cfg.Platform, RuntimeType: string(cfg.RuntimeType)}, nil
}

func (*starterRuntimeProvisioner) EnsurePlugins(runtimepkg.EnsurePluginsRequest) error {
	return nil
}
