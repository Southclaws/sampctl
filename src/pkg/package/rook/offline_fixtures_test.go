package rook

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	res "github.com/Southclaws/sampctl/src/resource"
)

var offlineCacheOnce sync.Once

func ensureOfflineFixtures(t *testing.T, cacheDir string) {
	t.Helper()

	var err error
	offlineCacheOnce.Do(func() {
		err = seedOfflineFixtures(cacheDir)
	})
	require.NoError(t, err)
}

func seedOfflineFixtures(cacheDir string) error {
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}

	if err := seedCachedPackageRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
		pawnpackage.Package{
			Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib",
		},
		nil,
		map[string]string{"include/pawn.inc": "// fixture"},
	); err != nil {
		return err
	}

	if err := seedCachedPackageRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
		pawnpackage.Package{
			Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib",
			Dependencies: []versioning.DependencyString{"pawn-lang/pawn-stdlib"},
		},
		nil,
		map[string]string{"a_samp.inc": "// fixture"},
	); err != nil {
		return err
	}

	if err := seedCachedPackageRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "AmyrAhmady", Repo: "samp-plugin-crashdetect"},
		pawnpackage.Package{
			Site: "github.com", User: "AmyrAhmady", Repo: "samp-plugin-crashdetect",
			Runtime: &run.Runtime{Plugins: []run.Plugin{"crashdetect"}},
			Resources: []res.Resource{{
				Name:     `^crashdetect-(.*)-linux.tar.gz$`,
				Platform: "linux",
				Archive:  true,
				Plugins:  []string{"crashdetect.so"},
			}},
		},
		[]string{"4.22.0"},
		map[string]string{"README.md": "fixture"},
	); err != nil {
		return err
	}
	if err := writeTarGz(filepath.Join(cacheDir, "packages", "AmyrAhmady", "samp-plugin-crashdetect", "default", "crashdetect-4.22-linux.tar.gz"), map[string]string{
		"crashdetect.so": "fixture plugin binary",
	}); err != nil {
		return err
	}

	if err := seedCachedPackageRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "YSI-Includes"},
		pawnpackage.Package{
			Site: "github.com", User: "pawn-lang", Repo: "YSI-Includes",
		},
		nil,
		map[string]string{"YSI_Core/y_utils.inc": "// fixture"},
	); err != nil {
		return err
	}

	if err := seedCachedPackageRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-errors", Tag: "1.2.3"},
		pawnpackage.Package{
			Site: "github.com", User: "Southclaws", Repo: "pawn-errors",
			Dependencies: []versioning.DependencyString{
				"pawn-lang/samp-stdlib",
				"AmyrAhmady/samp-plugin-crashdetect:v4.22",
			},
		},
		[]string{"1.2.3", "1.2.4", "1.3.0"},
		map[string]string{"pawn-errors.inc": "// fixture"},
	); err != nil {
		return err
	}

	return nil
}

func seedCachedPackageRepo(
	cacheDir string,
	meta versioning.DependencyMeta,
	pkg pawnpackage.Package,
	tags []string,
	extraFiles map[string]string,
) error {
	cachePath := meta.CachePath(cacheDir)
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return err
	}

	repo, err := git.PlainInit(cachePath, false)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(pkg, "", "\t")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cachePath, "pawn.json"), data, 0o644); err != nil {
		return err
	}
	if _, err := wt.Add("pawn.json"); err != nil {
		return err
	}

	for name, body := range extraFiles {
		path := filepath.Join(cachePath, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		if _, err := wt.Add(name); err != nil {
			return err
		}
	}

	_, err = wt.Commit("fixture", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	if err != nil {
		return err
	}

	if len(tags) == 0 {
		return nil
	}

	head, err := repo.Head()
	if err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := repo.CreateTag(tag, head.Hash(), nil); err != nil {
			return err
		}
	}

	return nil
}

func writeTarGz(path string, files map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	gzw := gzip.NewWriter(f)
	defer gzw.Close() //nolint:errcheck

	tw := tar.NewWriter(gzw)
	defer tw.Close() //nolint:errcheck

	for name, body := range files {
		contents := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}); err != nil {
			return err
		}
		if _, err := tw.Write(contents); err != nil {
			return err
		}
	}

	return nil
}
