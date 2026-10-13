package lockfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestCommitIntegrityWithVendoredFormerSubmodule(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, *git.Repository, string)
		clean  bool
	}{
		{name: "clean", clean: true},
		{name: "modified", change: func(t *testing.T, _ *git.Repository, dir string) {
			writeVendoredIntegrityFile(t, dir, "vendor/library.inc", "modified")
		}},
		{name: "deleted", change: func(t *testing.T, _ *git.Repository, dir string) {
			if err := os.Remove(filepath.Join(dir, "vendor/library.inc")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "untracked", change: func(t *testing.T, _ *git.Repository, dir string) {
			writeVendoredIntegrityFile(t, dir, "vendor/extra.inc", "untracked")
		}},
		{name: "ignored untracked", clean: true, change: func(t *testing.T, _ *git.Repository, dir string) {
			writeVendoredIntegrityFile(t, dir, "vendor/ignored.inc", "ignored")
		}},
		{name: "staged", change: func(t *testing.T, repo *git.Repository, dir string) {
			writeVendoredIntegrityFile(t, dir, "vendor/library.inc", "staged")
			worktree, err := repo.Worktree()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := worktree.Add("vendor/library.inc"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "outside vendor", change: func(t *testing.T, _ *git.Repository, dir string) {
			writeVendoredIntegrityFile(t, dir, "main.pwn", "modified")
		}},
		{name: "gitmodules modified", change: func(t *testing.T, _ *git.Repository, dir string) {
			writeVendoredIntegrityFile(t, dir, ".gitmodules", "")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, commit := seedGitRepo(t, map[string]string{
				"main.pwn":           "main() {}",
				"vendor/library.inc": "stock Library() {}",
				".gitignore":         "vendor/ignored.inc\n",
				".gitmodules":        "[submodule \"vendor\"]\n\tpath = vendor\n\turl = https://example.com/vendor.git\n",
			}, "")
			worktree, err := repo.Worktree()
			if err != nil {
				t.Fatal(err)
			}
			dir := worktree.Filesystem.Root()
			if test.change != nil {
				test.change(t, repo, dir)
			}
			before := snapshotIntegrityFiles(t, dir)
			clean, err := VerifyIntegrityContext(t.Context(), dir, CalculateCommitIntegrity(commit))
			if err != nil || clean != test.clean {
				t.Errorf("VerifyIntegrityContext(%s) = %t, %v, want %t, nil", test.name, clean, err, test.clean)
			}
			if after := snapshotIntegrityFiles(t, dir); !reflect.DeepEqual(after, before) {
				t.Errorf("VerifyIntegrityContext(%s) changed repository files or metadata", test.name)
			}
		})
	}
}

func writeVendoredIntegrityFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
