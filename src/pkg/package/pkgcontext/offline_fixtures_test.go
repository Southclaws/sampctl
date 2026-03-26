package pkgcontext

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	res "github.com/Southclaws/sampctl/src/pkg/package/resource"
	run "github.com/Southclaws/sampctl/src/pkg/runtime/config"
)

var (
	fixturePawnStdlibCommit string
	fixtureSampStdlibCommit string
)

func seedPkgContextFixtures(cacheDir string) error {
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}

	commit, err := seedPkgContextRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib"},
		pawnpackage.Package{
			Site: "github.com", User: "pawn-lang", Repo: "pawn-stdlib",
		},
		nil,
		"",
		map[string]string{"pawn.inc": "// fixture\n"},
	)
	if err != nil {
		return err
	}
	fixturePawnStdlibCommit = commit

	commit, err = seedPkgContextRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib"},
		pawnpackage.Package{
			Site: "github.com", User: "pawn-lang", Repo: "samp-stdlib",
			Dependencies: []versioning.DependencyString{"pawn-lang/pawn-stdlib"},
		},
		[]string{"0.3z-R4"},
		"",
		map[string]string{"a_samp.inc": "// fixture\n"},
	)
	if err != nil {
		return err
	}
	fixtureSampStdlibCommit = commit

	if _, err := seedPkgContextRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-errors"},
		pawnpackage.Package{
			Site: "github.com", User: "Southclaws", Repo: "pawn-errors",
		},
		nil,
		"v2",
		map[string]string{"pawn-errors.inc": "// fixture\n"},
	); err != nil {
		return err
	}

	if _, err := seedPkgContextRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "Southclaws", Repo: "pawn-requests"},
		pawnpackage.Package{
			Site: "github.com", User: "Southclaws", Repo: "pawn-requests",
			Dependencies: []versioning.DependencyString{"pawn-lang/samp-stdlib"},
			Resources: []res.Resource{{
				Name:     `^requests-.+-linux.tar.gz$`,
				Platform: "linux",
				Archive:  true,
				Includes: []string{"requests-.+/pawno/include"},
				Plugins:  []string{"requests-.+/plugins/requests.so"},
			}},
		},
		nil,
		"",
		map[string]string{"requests.inc": "// fixture\n"},
	); err != nil {
		return err
	}
	if err := writePkgContextTarGz(filepath.Join(cacheDir, "packages", "Southclaws", "pawn-requests", "default", "requests-1.0.0-linux.tar.gz"), map[string]string{
		"requests-1.0.0-linux/pawno/include/requests.inc": "// fixture\n",
		"requests-1.0.0-linux/plugins/requests.so":        "fixture plugin",
	}); err != nil {
		return err
	}

	if _, err := seedPkgContextRepo(cacheDir,
		versioning.DependencyMeta{Site: "github.com", User: "sampctl", Repo: "package-resource-test"},
		pawnpackage.Package{
			Site: "github.com", User: "sampctl", Repo: "package-resource-test",
			Dependencies: []versioning.DependencyString{"sampctl/samp-stdlib"},
			Runtime:      &run.Runtime{Plugins: []run.Plugin{"package-resource-test"}},
			Resources: []res.Resource{
				{
					Name:     `^test-.+-([Dd]ebian[0-9]?|[Ll]inux)\.tar\.gz$`,
					Platform: "linux",
					Archive:  true,
					Includes: []string{"test-.+/pawno/include"},
					Plugins:  []string{"test-.+/plugins/test.so"},
					Files: map[string]string{
						"test-.+/dependency.so": "dependency.so",
					},
				},
			},
		},
		nil,
		"",
		map[string]string{"test.pwn": "main() {}\n"},
	); err != nil {
		return err
	}
	return writePkgContextTarGz(filepath.Join(cacheDir, "packages", "sampctl", "package-resource-test", "default", "test-1.0.0-linux.tar.gz"), map[string]string{
		"test-1.0.0-linux/pawno/include/include.inc": "// fixture\n",
		"test-1.0.0-linux/plugins/test.so":           "fixture plugin",
		"test-1.0.0-linux/dependency.so":             "fixture dependency",
	})
}

func seedPkgContextRepo(cacheDir string, meta versioning.DependencyMeta, pkg pawnpackage.Package, tags []string, branch string, files map[string]string) (string, error) {
	cachePath := meta.CachePath(cacheDir)
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return "", err
	}

	repo, err := git.PlainInit(cachePath, false)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(pkg, "", "\t")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(cachePath, "pawn.json"), data, 0o644); err != nil {
		return "", err
	}
	if _, err := wt.Add("pawn.json"); err != nil {
		return "", err
	}

	for name, body := range files {
		path := filepath.Join(cachePath, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return "", err
		}
		if _, err := wt.Add(name); err != nil {
			return "", err
		}
	}

	hash, err := wt.Commit("fixture", &git.CommitOptions{
		Author:    &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
		Committer: &object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(100, 0)},
	})
	if err != nil {
		return "", err
	}

	for _, tag := range tags {
		if _, err := repo.CreateTag(tag, hash, nil); err != nil {
			return "", err
		}
	}
	if branch != "" {
		if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), hash)); err != nil {
			return "", err
		}
	}

	return hash.String(), nil
}

func writePkgContextTarGz(path string, files map[string]string) error {
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
