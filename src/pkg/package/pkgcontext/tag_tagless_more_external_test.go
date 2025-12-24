package pkgcontext_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
)

func TestTagTaglessDependencies_DoesNotModifyAlreadyVersioned(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	sha := "0123456789abcdef0123456789abcdef01234567"
	cfg := map[string]any{
		"entry":        "test.pwn",
		"output":       "test.amx",
		"dependencies": []string{"u/r:1.0.0", "u/r@main", "u/r#" + sha},
	}
	initialBytes, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), initialBytes, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux"}
	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.NoError(t, err)
	require.False(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)
	require.Equal(t, string(initialBytes), string(finalBytes))
}

func TestTagTaglessDependencies_TagsMultipleAndDevDependencies(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	seedCachedRepoWithTag(t, cacheDir, "u1", "r1", "1.0.0")
	seedCachedRepoWithTag(t, cacheDir, "u2", "r2", "2.3.4")
	seedCachedRepoWithTag(t, cacheDir, "u3", "r3", "0.9.0")

	cfg := map[string]any{
		"entry":            "test.pwn",
		"output":           "test.amx",
		"dependencies":     []string{"u1/r1", "u2/r2"},
		"dev_dependencies": []string{"u3/r3"},
	}
	b, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), b, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux"}
	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.NoError(t, err)
	require.True(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)
	var final map[string]any
	require.NoError(t, json.Unmarshal(finalBytes, &final))

	depsAny := final["dependencies"].([]any)
	require.Equal(t, []any{"u1/r1:1.0.0", "u2/r2:2.3.4"}, depsAny)

	devAny := final["dev_dependencies"].([]any)
	require.Equal(t, []any{"u3/r3:0.9.0"}, devAny)
}

func TestTagTaglessDependencies_TagsDependencyWithPath(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	seedCachedRepoWithTag(t, cacheDir, "u", "r", "1.2.3")

	cfg := map[string]any{
		"entry":        "test.pwn",
		"output":       "test.amx",
		"dependencies": []string{"u/r/include"},
	}
	b, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), b, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux"}
	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.NoError(t, err)
	require.True(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)
	var final map[string]any
	require.NoError(t, json.Unmarshal(finalBytes, &final))

	depsAny := final["dependencies"].([]any)
	require.Equal(t, []any{"u/r/include:1.2.3"}, depsAny)
}

func TestTagTaglessDependencies_UsesGitHubFallbackWhenNoTagsInCache(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	// Seed cached repo with NO tags.
	seedCachedRepoNoTags(t, cacheDir, "testuser", "testrepo")

	// Fake GitHub API for releases.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/testuser/testrepo/releases" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"tag_name":"stable","draft":false,"prerelease":false}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	hc := srv.Client()
	gh := github.NewClient(hc)
	base, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = base

	cfg := map[string]any{
		"entry":        "test.pwn",
		"output":       "test.amx",
		"dependencies": []string{"testuser/testrepo"},
	}
	b, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), b, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux", GitHub: gh}
	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.NoError(t, err)
	require.True(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)
	var final map[string]any
	require.NoError(t, json.Unmarshal(finalBytes, &final))

	depsAny := final["dependencies"].([]any)
	require.Equal(t, []any{"testuser/testrepo:stable"}, depsAny)
}

func TestTagTaglessDependencies_DoesNotTagWhenNoTagsAndNoGitHub(t *testing.T) {
	cacheDir := t.TempDir()
	projectDir := t.TempDir()

	seedCachedRepoNoTags(t, cacheDir, "u", "r")

	cfg := map[string]any{
		"entry":        "test.pwn",
		"output":       "test.amx",
		"dependencies": []string{"u/r"},
	}
	initialBytes, err := json.MarshalIndent(cfg, "", "\t")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "pawn.json"), initialBytes, 0o644))

	pkg, err := pawnpackage.PackageFromDir(projectDir)
	require.NoError(t, err)
	pkg.Parent = true
	pkg.LocalPath = projectDir
	pkg.DependencyMeta = versioning.DependencyMeta{User: "local", Repo: "project"}

	pcx := pkgcontext.PackageContext{Package: pkg, CacheDir: cacheDir, Platform: "linux"}
	updated, err := pcx.TagTaglessDependencies(context.Background(), false)
	require.NoError(t, err)
	require.False(t, updated)

	finalBytes, err := os.ReadFile(filepath.Join(projectDir, "pawn.json"))
	require.NoError(t, err)
	require.Equal(t, string(initialBytes), string(finalBytes))
}

func seedCachedRepoWithTag(t *testing.T, cacheDir, user, repoName, tag string) {
	meta := versioning.DependencyMeta{User: user, Repo: repoName}
	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), []byte("{}"), 0o644))
	_, err = wt.Add("pawn.json")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "file.txt"), []byte("one"), 0o644))
	_, err = wt.Add("file.txt")
	require.NoError(t, err)

	_, err = wt.Commit("c1", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	require.NoError(t, err)

	head, err := repo.Head()
	require.NoError(t, err)
	_, err = repo.CreateTag(tag, head.Hash(), nil)
	require.NoError(t, err)
}

func seedCachedRepoNoTags(t *testing.T, cacheDir, user, repoName string) {
	meta := versioning.DependencyMeta{User: user, Repo: repoName}
	cachePath := meta.CachePath(cacheDir)
	require.NoError(t, os.MkdirAll(cachePath, 0o755))

	repo, err := git.PlainInit(cachePath, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), []byte("{}"), 0o644))
	_, err = wt.Add("pawn.json")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "file.txt"), []byte("one"), 0o644))
	_, err = wt.Add("file.txt")
	require.NoError(t, err)

	_, err = wt.Commit("c1", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	require.NoError(t, err)
}
