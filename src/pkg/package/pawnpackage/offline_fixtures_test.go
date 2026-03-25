package pawnpackage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func seedPawnPackageFixtureCache(t *testing.T) string {
	t.Helper()

	cacheDir := filepath.Join(t.TempDir(), "cache")
	requireDir(t, cacheDir)
	seedPawnRepo(t, cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
		pawnpackage.Package{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
		map[string]string{"pawn.inc": "// fixture\n"},
	)
	seedPawnRepo(t, cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
		pawnpackage.Package{
			Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib",
			Dependencies: []versioning.DependencyString{"pawn-lang/pawn-stdlib"},
		},
		map[string]string{"a_samp.inc": "// fixture\n"},
	)
	seedPawnRepo(t, cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-uuid"},
		pawnpackage.Package{
			Site: "github.com", User: "Southclaws", Repo: "pawn-uuid",
			Dependencies: []versioning.DependencyString{"pawn-lang/samp-stdlib"},
		},
		map[string]string{"uuid.inc": "// fixture\n"},
	)
	return cacheDir
}

func seedPawnRepo(t *testing.T, cacheDir string, meta versioning.DependencyMeta, pkg pawnpackage.Package, files map[string]string) {
	t.Helper()

	cachePath := meta.CachePath(cacheDir)
	requireDir(t, cachePath)
	repo, err := git.PlainInit(cachePath, false)
	if err != nil {
		t.Fatalf("init repo %s: %v", cachePath, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree %s: %v", cachePath, err)
	}
	data, err := json.MarshalIndent(pkg, "", "\t")
	if err != nil {
		t.Fatalf("marshal package %s: %v", cachePath, err)
	}
	if err := os.WriteFile(filepath.Join(cachePath, "pawn.json"), data, 0o644); err != nil {
		t.Fatalf("write pawn.json %s: %v", cachePath, err)
	}
	if _, err := wt.Add("pawn.json"); err != nil {
		t.Fatalf("add pawn.json %s: %v", cachePath, err)
	}
	for name, body := range files {
		path := filepath.Join(cachePath, name)
		requireDir(t, filepath.Dir(path))
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write file %s: %v", path, err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("add file %s: %v", name, err)
		}
	}
	if _, err := wt.Commit("fixture", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	}); err != nil {
		t.Fatalf("commit repo %s: %v", cachePath, err)
	}
}

func requireDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func newPackageTestCompilerDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "pawncc")
	script := `#!/bin/sh
output=""
for arg in "$@"; do
  case "$arg" in
    -o*) output="${arg#-o}" ;;
  esac
done
if [ -n "$output" ]; then
  mkdir -p "$(dirname "$output")"
  : > "$output"
fi
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake compiler: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "libpawnc.so"), []byte("fixture"), 0o644); err != nil {
		t.Fatalf("write fake compiler library: %v", err)
	}
	return dir
}
