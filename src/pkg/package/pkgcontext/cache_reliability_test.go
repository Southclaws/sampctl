package pkgcontext

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/gofrs/flock"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func cacheFixtureRepository(t *testing.T, path, definition string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(path, false)
	if err != nil {
		t.Fatal(err)
	}
	name, contents := "include.inc", "// include\n"
	if definition != "" {
		name, contents = "pawn.json", definition
	}
	if err := os.WriteFile(filepath.Join(path, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("fixture", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.invalid", When: time.Unix(100, 0)}}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestDependencyGraphPreservesFailuresAfterIndependentSuccess(t *testing.T) {
	cacheDir := t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "valid"}
	cacheFixtureRepository(t, meta.CachePath(cacheDir), "")
	failure := errors.New("remote unavailable")
	store := &fakeRepositoryStore{
		openFn:  git.PlainOpen,
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) { return nil, failure },
	}
	pcx := PackageContext{
		Package:         pawnpackage.Package{Parent: true, LocalPath: t.TempDir(), User: "fixture", Repo: "project", Dependencies: []versioning.DependencyString{"fixture/missing-a", "fixture/missing-b", "fixture/valid"}},
		PackageServices: PackageServices{CacheDir: cacheDir, RepoStore: store},
	}
	err := pcx.EnsureDependenciesCached()
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "missing-a") || !strings.Contains(err.Error(), "missing-b") {
		t.Errorf("EnsureDependenciesCached() = %v, want both dependency failures and original cause", err)
	}
	if len(pcx.AllDependencies) != 1 || pcx.AllDependencies[0].Repo != "valid" {
		t.Errorf("AllDependencies = %v, want successful independent dependency", pcx.AllDependencies)
	}
}

func TestDependencyGraphRejectsInvalidManifest(t *testing.T) {
	for _, definition := range []string{"{", `{"dependencies":["invalid dependency"]}`} {
		t.Run(definition, func(t *testing.T) {
			cacheDir := t.TempDir()
			meta := versioning.DependencyMeta{User: "fixture", Repo: "invalid"}
			cacheFixtureRepository(t, meta.CachePath(cacheDir), definition)
			pcx := PackageContext{
				Package:         pawnpackage.Package{Parent: true, LocalPath: t.TempDir(), User: "fixture", Repo: "project", Dependencies: []versioning.DependencyString{"fixture/invalid"}},
				PackageServices: PackageServices{CacheDir: cacheDir},
			}
			if err := pcx.EnsureDependenciesCached(); err == nil {
				t.Error("EnsureDependenciesCached(invalid metadata) = nil, want error")
			}
		})
	}
}

func TestDependencyGraphCancellationStopsFurtherDependencies(t *testing.T) {
	store := &fakeRepositoryStore{
		openFn:  git.PlainOpen,
		cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) { return nil, context.Canceled },
	}
	pcx := PackageContext{
		Package:         pawnpackage.Package{Parent: true, LocalPath: t.TempDir(), User: "fixture", Repo: "project", Dependencies: []versioning.DependencyString{"fixture/first", "fixture/second"}},
		PackageServices: PackageServices{CacheDir: t.TempDir(), RepoStore: store},
	}
	err := pcx.EnsureDependenciesCached()
	if !errors.Is(err, context.Canceled) || store.cloneCalls != 1 {
		t.Errorf("EnsureDependenciesCached() = %v after %d clones, want cancellation after first clone", err, store.cloneCalls)
	}
}

func TestCloneFailurePreservesExistingRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repository")
	cacheFixtureRepository(t, path, "")
	if err := os.WriteFile(filepath.Join(path, "untracked.txt"), []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("download interrupted")
	store := &fakeRepositoryStore{cloneFn: func(stage string, _ bool, _ *git.CloneOptions) (*git.Repository, error) {
		if stage == path {
			t.Error("clone destination = existing repository, want separate staging directory")
		}
		if err := os.MkdirAll(stage, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "partial"), []byte("incomplete"), 0o600); err != nil {
			t.Fatal(err)
		}
		return nil, failure
	}}
	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store}}
	_, err := pcx.recoverByReclone("https://example.invalid/repository", path, "", false, false)
	if !errors.Is(err, failure) {
		t.Errorf("recoverByReclone() = %v, want download cause", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "untracked.txt"))
	if err != nil || string(data) != "keep me" {
		t.Errorf("ReadFile(existing file) = %q, %v, want preserved content", data, err)
	}
	if valid, err := ValidateRepository(path); !valid || err != nil {
		t.Errorf("ValidateRepository(previous clone) = %v, %v, want valid repository", valid, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("ReadDir() = %v, %v, want no remaining staging directory", entries, err)
	}
}

func TestEnsureRepoExistsPreservesUnmanagedDirectory(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "personal.txt"), []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &fakeRepositoryStore{openFn: git.PlainOpen, cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
		return nil, errors.New("remote failed")
	}}
	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store}}
	_, err := pcx.ensureRepoExists(repoEnsureRequest{From: "https://example.invalid/repository", To: path})
	if err == nil || store.cloneCalls != 0 {
		t.Errorf("ensureRepoExists(unmanaged path) = %v after %d clones, want refusal before clone", err, store.cloneCalls)
	}
	if data, err := os.ReadFile(filepath.Join(path, "personal.txt")); err != nil || string(data) != "keep me" {
		t.Errorf("ReadFile(personal.txt) = %q, %v, want preserved content", data, err)
	}
}

func TestGitErrorWrappingPreservesCauseAndRedactsDetails(t *testing.T) {
	for _, cause := range []error{context.Canceled, transport.ErrAuthenticationRequired, transport.ErrRepositoryNotFound, fmt.Errorf("git failed with https://user:secret-token@example.invalid/repo")} {
		err := WrapGitError(cause, versioning.DependencyMeta{User: "fixture", Repo: "repository"})
		if !errors.Is(err, cause) {
			t.Errorf("WrapGitError(%v) = %v, want preserved cause", cause, err)
		}
		if !strings.Contains(err.Error(), "fixture/repository") || strings.Contains(err.Error(), "secret-token") {
			t.Errorf("WrapGitError() = %q, want dependency context without credentials", err)
		}
	}
}

func TestCloneRetryClassification(t *testing.T) {
	for _, test := range []struct {
		name     string
		cause    error
		attempts int
	}{
		{name: "transient network", cause: &net.DNSError{Err: "timeout", IsTimeout: true}, attempts: 3},
		{name: "HTTP unavailable", cause: plumbing.NewUnexpectedError(&gitHTTP.Err{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}}), attempts: 3},
		{name: "missing repository", cause: transport.ErrRepositoryNotFound, attempts: 1},
		{name: "permission denied", cause: os.ErrPermission, attempts: 1},
		{name: "canceled", cause: context.Canceled, attempts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeRepositoryStore{cloneFn: func(path string, _ bool, _ *git.CloneOptions) (*git.Repository, error) {
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "partial"), []byte("partial"), 0o600); err != nil {
					t.Fatal(err)
				}
				return nil, test.cause
			}}
			pcx := PackageContext{PackageServices: PackageServices{RepoStore: store}}
			path := filepath.Join(t.TempDir(), "repository")
			_, err := pcx.cloneRepository("https://example.invalid/repository", path, "", false, false)
			if !errors.Is(err, test.cause) || store.cloneCalls != test.attempts {
				t.Errorf("cloneRepository() = %v after %d attempts, want cause %v after %d", err, store.cloneCalls, test.cause, test.attempts)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Stat(final path) = %v, want no partial repository", err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 0 {
				t.Errorf("ReadDir() = %v, %v, want no clone staging directories", entries, err)
			}
		})
	}
}

func TestCloneValidationFailurePreservesPreviousRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repository")
	cacheFixtureRepository(t, path, "")
	store := &fakeRepositoryStore{cloneFn: func(stage string, _ bool, _ *git.CloneOptions) (*git.Repository, error) {
		return cacheFixtureRepository(t, stage, ""), nil
	}}
	invalid := errors.New("invalid downloaded repository")
	health := &fakeRepositoryHealth{validateFn: func(string) (bool, error) { return false, invalid }}
	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store, RepoHealth: health}}
	_, err := pcx.cloneRepository("https://example.invalid/repository", path, "", false, false)
	if !errors.Is(err, invalid) {
		t.Errorf("cloneRepository() = %v, want validation error", err)
	}
	if valid, err := ValidateRepository(path); !valid || err != nil {
		t.Errorf("ValidateRepository(previous) = %v, %v, want preserved valid repository", valid, err)
	}
}

func TestClonePromotionOpenFailureRestoresPreviousRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repository")
	previous := cacheFixtureRepository(t, path, "")
	head, err := previous.Head()
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("failed to reopen promoted repository")
	store := &fakeRepositoryStore{
		openFn: func(string) (*git.Repository, error) { return nil, failure },
		cloneFn: func(stage string, _ bool, _ *git.CloneOptions) (*git.Repository, error) {
			return cacheFixtureRepository(t, stage, `{"dependencies":["fixture/new"]}`), nil
		},
	}
	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store}}
	_, err = pcx.cloneRepository("https://example.invalid/repository", path, "", false, false)
	if !errors.Is(err, failure) {
		t.Errorf("cloneRepository() = %v, want reopen error", err)
	}
	repo, err := git.PlainOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := repo.Head()
	if err != nil || restored.Hash() != head.Hash() {
		t.Errorf("restored Head() = %v, %v, want original %v", restored, err, head)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("ReadDir() = %v, %v, want only restored repository", entries, err)
	}
}

func TestEnsureDependencyCacheRejectsTraversalAndSymlinks(t *testing.T) {
	for _, meta := range []versioning.DependencyMeta{
		{User: "..", Repo: "repository"},
		{User: "fixture", Repo: "../outside"},
		{User: "fixture", Repo: "repository", Branch: "../../outside"},
	} {
		pcx := PackageContext{PackageServices: PackageServices{CacheDir: t.TempDir()}}
		if _, err := pcx.EnsureDependencyCached(meta, false); err == nil {
			t.Errorf("EnsureDependencyCached(%s) = nil, want path traversal rejected", meta)
		}
	}
	cacheDir, external := t.TempDir(), t.TempDir()
	marker := filepath.Join(external, "personal.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(cacheDir, "packages")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir}}
	_, err := pcx.EnsureDependencyCached(versioning.DependencyMeta{User: "fixture", Repo: "repository"}, false)
	if err == nil {
		t.Error("EnsureDependencyCached(symlink cache subtree) = nil, want refusal")
	}
	if contents, err := os.ReadFile(marker); err != nil || string(contents) != "keep me" {
		t.Errorf("ReadFile(external marker) = %q, %v, want preserved content", contents, err)
	}
}

func TestRepositoryLockWaitHonorsCancellation(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "repository")
	lock := flock.New(filepath.Join(parent, ".repository.sampctl.lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	pcx := PackageContext{}
	_, err := pcx.ensureRepoExists(repoEnsureRequest{Context: ctx, From: "https://example.invalid/repository", To: path})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ensureRepoExists(locked path) = %v, want deadline exceeded", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat(repository) = %v, want no operation performed", err)
	}
}

func TestRepositoryMetadataSymlinkIsNeverFollowed(t *testing.T) {
	cacheDir, external := t.TempDir(), t.TempDir()
	meta := versioning.DependencyMeta{User: "fixture", Repo: "repository"}
	path := meta.CachePath(cacheDir)
	cacheFixtureRepository(t, path, "")
	marker := filepath.Join(external, "HEAD")
	if err := os.WriteFile(marker, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(path, ".git", "untrusted")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	store := &fakeRepositoryStore{openFn: func(string) (*git.Repository, error) {
		t.Error("repository with symbolic links should be rejected before opening")
		return nil, git.ErrRepositoryNotExists
	}}
	pcx := PackageContext{PackageServices: PackageServices{CacheDir: cacheDir, RepoStore: store}}
	_, err := pcx.EnsureDependencyCached(meta, true)
	if !errors.Is(err, os.ErrInvalid) {
		t.Errorf("EnsureDependencyCached(metadata symlink) = %v, want refusal", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep me" {
		t.Errorf("ReadFile(external HEAD) = %q, %v, want preserved content", data, err)
	}
}

func TestFailedCacheRefreshPreservesCommittedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repository")
	previous := cacheFixtureRepository(t, path, "")
	head, err := previous.Head()
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("failed after staging cache update")
	store := &fakeRepositoryStore{openFn: func(openPath string) (*git.Repository, error) {
		repo, err := git.PlainOpen(openPath)
		if err != nil {
			return nil, err
		}
		if openPath == path {
			return repo, nil
		}
		if err := os.WriteFile(filepath.Join(openPath, "include.inc"), []byte("incomplete refresh"), 0o600); err != nil {
			t.Fatal(err)
		}
		return nil, failure
	}}
	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store}}
	_, err = pcx.ensureRepoExists(repoEnsureRequest{From: "https://example.invalid/repository", To: path, ForceUpdate: true})
	if !errors.Is(err, failure) {
		t.Errorf("ensureRepoExists(force update) = %v, want staged update error", err)
	}
	contents, err := os.ReadFile(filepath.Join(path, "include.inc"))
	if err != nil || string(contents) != "// include\n" {
		t.Errorf("ReadFile(previous worktree) = %q, %v, want original contents", contents, err)
	}
	repo, err := git.PlainOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	finalHead, err := repo.Head()
	if err != nil || finalHead.Hash() != head.Hash() {
		t.Errorf("Head() = %v, %v, want previous %v", finalHead, err, head)
	}
}

func TestRepositoryValidationAndCleanupRejectUnmanagedPaths(t *testing.T) {
	path := t.TempDir()
	marker := filepath.Join(path, "personal.txt")
	if err := os.WriteFile(marker, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CleanInvalidRepository(path); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("CleanInvalidRepository(unmanaged directory) = %v, want refusal", err)
	}
	if contents, err := os.ReadFile(marker); err != nil || string(contents) != "keep me" {
		t.Errorf("ReadFile(personal file) = %q, %v, want preserved data", contents, err)
	}
}

func TestCacheMetadataReadWaitHonorsCancellation(t *testing.T) {
	meta := versioning.DependencyMeta{User: "fixture", Repo: "repository"}
	cacheDir := t.TempDir()
	path := meta.CachePath(cacheDir)
	cacheFixtureRepository(t, path, "")
	unlock, err := lockRepositoryPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unlock(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	pcx := PackageContext{}
	_, err = pcx.packageFromCachedRevisionContext(ctx, meta, path)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("packageFromCachedRevisionContext(locked cache) = %v, want deadline", err)
	}
}

func TestCloneUnexpectedErrorPreservesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var transportFailure error
	store := &fakeRepositoryStore{cloneFn: func(string, bool, *git.CloneOptions) (*git.Repository, error) {
		cancel()
		transportFailure = plumbing.NewUnexpectedError(ctx.Err())
		return nil, transportFailure
	}}
	pcx := PackageContext{PackageServices: PackageServices{RepoStore: store}}
	path := filepath.Join(t.TempDir(), "repository")
	_, err := pcx.cloneRepositoryContext(repoEnsureRequest{Context: ctx, From: "https://example.invalid/repository", To: path})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, transportFailure) || store.cloneCalls != 1 {
		t.Errorf("cloneRepositoryContext() = %v after %d attempts, want cancellation and original transport cause after 1", err, store.cloneCalls)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 0 {
		t.Errorf("ReadDir() = %v, %v, want no staging artifacts after cancellation", entries, err)
	}
}

func TestSharedRepositoryValidationRejectsMetadataSymlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repository")
	cacheFixtureRepository(t, path, "")
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(path, ".git", "untrusted")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if valid, err := ValidateRepository(path); valid || !errors.Is(err, os.ErrInvalid) {
		t.Errorf("ValidateRepository(metadata symlink) = %v, %v, want invalid unsafe path", valid, err)
	}
	if err := RepairRepository(path); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("RepairRepository(metadata symlink) = %v, want refusal", err)
	}
	if err := CleanInvalidRepository(path); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("CleanInvalidRepository(metadata symlink) = %v, want refusal", err)
	}
	if _, err := os.Lstat(filepath.Join(path, ".git", "untrusted")); err != nil {
		t.Errorf("Lstat(metadata symlink) = %v, want preserved repository for manual recovery", err)
	}
}
