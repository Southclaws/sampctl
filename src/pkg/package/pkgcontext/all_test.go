package pkgcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
)

var (
	gh      *github.Client
	gitAuth transport.AuthMethod
)

func TestMain(m *testing.M) {
	err := os.MkdirAll("./tests/cache", 0o700)
	if err != nil {
		panic(err)
	}
	if err := seedPkgContextFixtures(filepath.Clean("./tests/cache")); err != nil {
		panic(err)
	}
	if err := stripPkgContextFixtureCacheRemotes(filepath.Clean("./tests/cache")); err != nil {
		panic(err)
	}

	print.SetVerbose()

	os.Exit(m.Run())
}

func stripPkgContextFixtureCacheRemotes(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Name() != ".git" {
			return nil
		}

		repo, err := git.PlainOpen(filepath.Dir(path))
		if err != nil {
			return err
		}
		if err := repo.DeleteRemote("origin"); err != nil && !strings.Contains(err.Error(), "remote not found") {
			return err
		}

		return filepath.SkipDir
	})
}
