package commands

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/kirsle/configdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/urfave/cli.v1"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/lockfile"
	"github.com/Southclaws/sampctl/src/pkg/package/resource"
)

func TestPackageBuildResolvesLockfileBeforePreparingIncludes(t *testing.T) {
	for _, name := range []string{"locked", "no-lock", "invalid lockfile"} {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(configdir.Refresh)
			configHome := t.TempDir()
			t.Setenv("HOME", configHome)
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("APPDATA", configHome)
			configdir.Refresh()
			cache, err := fs.ConfigDir()
			require.NoError(t, err)

			root := t.TempDir()
			parent, err := versioning.DependencyString("fixture/parent@release").Explode()
			require.NoError(t, err)
			child, err := versioning.DependencyString("fixture/child").Explode()
			require.NoError(t, err)
			parentPath := parent.CachePath(cache)
			pinned := newCheckDependencyRepo(t, parentPath, `{"repo":"parent","dependencies":["fixture/child"]}`)
			childDefinition := `{"repo":"child","resources":[{"name":"child.so","platform":"linux","includes":["include"]}]}`
			childCommit := newCheckDependencyRepo(t, child.CachePath(cache), childDefinition)

			repo, err := git.PlainOpen(parentPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(parentPath, "pawn.json"), []byte(`{"repo":"parent"}`), 0o644))
			worktree, err := repo.Worktree()
			require.NoError(t, err)
			_, err = worktree.Add("pawn.json")
			require.NoError(t, err)
			head, err := worktree.Commit("advance cache", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(500, 0)}})
			require.NoError(t, err)
			require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("release"), head)))

			lf := lockfile.New("test")
			lf.AddDependency(lockfile.DependencyKey(parent), lockfile.LockedDependency{Site: parent.Site, User: parent.User, Repo: parent.Repo, Constraint: "@release", Commit: pinned})
			lf.AddDependency(lockfile.DependencyKey(child), lockfile.LockedDependency{Site: child.Site, User: child.User, Repo: child.Repo, Commit: childCommit, Transitive: true})
			require.NoError(t, lockfile.Save(root, lf))
			if name == "invalid lockfile" {
				require.NoError(t, os.WriteFile(filepath.Join(root, lockfile.Filename), []byte("{"), 0o600))
			}

			require.NoError(t, os.WriteFile(filepath.Join(root, "pawn.json"), []byte(`{"user":"fixture","repo":"root","entry":"main.pwn","output":"main.amx","dependencies":["fixture/parent@release"],"builds":[{"name":"default","compiler":{"path":"compiler"}}]}`), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "main.pwn"), []byte("main() {}"), 0o644))
			compilerDir := filepath.Join(root, "compiler")
			require.NoError(t, os.Mkdir(compilerDir, 0o755))
			binary := "pawncc"
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			require.NoError(t, os.WriteFile(filepath.Join(compilerDir, binary), nil, 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "dependencies", "parent"), 0o755))
			childDir := filepath.Join(root, "dependencies", "child")
			require.NoError(t, os.MkdirAll(childDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(childDir, "pawn.json"), []byte(childDefinition), 0o644))
			res := resource.Resource{Name: "child.so"}
			includePath := filepath.Join(root, "dependencies", res.Path("child"))
			require.NoError(t, os.MkdirAll(includePath, 0o755))

			app := newTestCLIContext("test").App
			flags := flag.NewFlagSet("build", flag.ContinueOnError)
			flags.String("dir", root, "")
			flags.String("platform", "linux", "")
			flags.Bool("dryRun", true, "")
			flags.Bool("no-lock", name == "no-lock", "")
			ctx := cli.NewContext(app, flags, nil)
			var buildErr error
			output := captureStdout(t, func() { buildErr = packageBuild(ctx) })
			require.NoError(t, buildErr)
			if name == "locked" {
				assert.Contains(t, output, "-i"+includePath)
			} else {
				assert.NotContains(t, output, "-i"+includePath)
			}
		})
	}
}
