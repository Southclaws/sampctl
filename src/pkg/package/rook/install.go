package rook

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pkgcontext"
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
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return errors.Wrap(err, "failed to create directory for clone")
	}

	if !util.DirEmpty(dir) {
		dir = filepath.Join(dir, meta.Repo)
	}

	print.Verb("cloning package", meta, "to", dir)

	repo, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:   meta.URL(),
		Depth: 1000, // TODO: We might want to consider removing depth for better reliability, or add a configurable option
	})
	if err != nil {
		return errors.Wrap(err, "failed to clone package repository")
	}

	valid, validationErr := pkgcontext.ValidateRepository(dir)
	if validationErr != nil || !valid {
		print.Verb("cloned repository failed validation, cleaning up")
		os.RemoveAll(dir)
		if validationErr != nil {
			return errors.Wrap(validationErr, "cloned repository is invalid")
		}
		return errors.New("cloned repository failed validation")
	}

	_, err = repo.Head()
	if err != nil {
		print.Verb("cloned repository has invalid HEAD, cleaning up")
		os.RemoveAll(dir)
		return errors.Wrap(err, "cloned repository has invalid HEAD")
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
