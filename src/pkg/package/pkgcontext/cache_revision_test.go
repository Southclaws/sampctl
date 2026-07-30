package pkgcontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func TestDependencyGraphUsesPinnedRevisionDefinition(t *testing.T) {
	tests := []struct {
		name string
		pin  func(versioning.DependencyMeta, string) versioning.DependencyMeta
	}{
		{
			name: "tag",
			pin: func(meta versioning.DependencyMeta, _ string) versioning.DependencyMeta {
				meta.Tag = "1.0.0"
				return meta
			},
		},
		{
			name: "commit",
			pin: func(meta versioning.DependencyMeta, commit string) versioning.DependencyMeta {
				meta.Commit = commit
				return meta
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			meta := versioning.DependencyMeta{Site: "github.com", User: "fixture", Repo: "library"}
			cachePath := meta.CachePath(cacheDir)
			require.NoError(t, os.MkdirAll(cachePath, 0o700))

			repo, err := git.PlainInit(cachePath, false)
			require.NoError(t, err)
			wt, err := repo.Worktree()
			require.NoError(t, err)

			require.NoError(t, os.WriteFile(filepath.Join(cachePath, "library.inc"), []byte("// legacy\n"), 0o600))
			_, err = wt.Add("library.inc")
			require.NoError(t, err)
			legacyCommit, err := wt.Commit("legacy release", &git.CommitOptions{
				Author: &object.Signature{
					Name:  "test",
					Email: "test@example.com",
					When:  time.Unix(100, 0),
				},
			})
			require.NoError(t, err)
			_, err = repo.CreateTag("1.0.0", legacyCommit, nil)
			require.NoError(t, err)

			definition := `{"dependencies":["openmultiplayer/omp-stdlib"]}`
			require.NoError(t, os.WriteFile(filepath.Join(cachePath, "pawn.json"), []byte(definition), 0o600))
			_, err = wt.Add("pawn.json")
			require.NoError(t, err)
			_, err = wt.Commit("add current dependencies", &git.CommitOptions{
				Author: &object.Signature{
					Name:  "test",
					Email: "test@example.com",
					When:  time.Unix(200, 0),
				},
			})
			require.NoError(t, err)

			pinned := test.pin(meta, legacyCommit.String())
			dependency := pinned.User + "/" + pinned.Repo
			if pinned.Tag != "" {
				dependency += ":" + pinned.Tag
			} else {
				dependency += "#" + pinned.Commit
			}
			pcx := PackageContext{
				Package: pawnpackage.Package{
					Parent:       true,
					LocalPath:    t.TempDir(),
					User:         "fixture",
					Repo:         "project",
					Dependencies: []versioning.DependencyString{versioning.DependencyString(dependency)},
				},
				PackageServices: PackageServices{
					CacheDir: cacheDir,
					Platform: "linux",
				},
			}

			require.NoError(t, pcx.EnsureDependenciesCached())
			require.Equal(t, []versioning.DependencyMeta{pinned}, pcx.AllDependencies)
		})
	}
}
