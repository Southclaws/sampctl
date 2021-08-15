package rook

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/pkgcontext"
	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Get simply performs a git clone of the given package to the specified directory then ensures it
func Get(
	ctx context.Context,
	gh *github.Client,
	meta versioning.DependencyMeta,
	dir string,
	auth transport.AuthMethod,
	platform,
	cacheDir string,
) (err error) {
	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return errors.Wrap(err, "failed to create directory for clone")
	}

	if !util.DirEmpty(dir) {
		dir = filepath.Join(dir, meta.Repo)
	}

	print.Verb("cloning package", meta, "to", dir)

	_, err = git.PlainClone(dir, false, &git.CloneOptions{
		URL: meta.URL(),
	})
	if err != nil {
		return errors.Wrap(err, "failed to clone package repository")
	}

	print.Verb("ensuring cloned package", meta, "to", dir)
	pcx, err := pkgcontext.NewPackageContext(gh, auth, true, dir, platform, cacheDir, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to read cloned repository as Pawn package")
	}

	err = pcx.EnsureDependencies(ctx, true)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies for cloned package")
	}

	return nil
}
