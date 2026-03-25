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

// GetOptions describes a package clone-and-ensure operation.
type GetOptions struct {
	Context  context.Context
	GitHub   *github.Client
	Meta     versioning.DependencyMeta
	Dir      string
	Auth     transport.AuthMethod
	Platform string
	CacheDir string
}

// Get simply performs a git clone of the given package to the specified directory then ensures it.
func Get(options GetOptions) (err error) {
	err = os.MkdirAll(options.Dir, 0o700)
	if err != nil {
		return errors.Wrap(err, "failed to create directory for clone")
	}

	if !util.DirEmpty(options.Dir) {
		options.Dir = filepath.Join(options.Dir, options.Meta.Repo)
	}

	print.Verb("cloning package", options.Meta, "to", options.Dir)

	repo, err := git.PlainClone(options.Dir, false, &git.CloneOptions{
		URL:   options.Meta.URL(),
		Depth: 1000, // TODO: We might want to consider removing depth for better reliability, or add a configurable option
	})
	if err != nil {
		return errors.Wrap(err, "failed to clone package repository")
	}

	valid, validationErr := pkgcontext.ValidateRepository(options.Dir)
	if validationErr != nil || !valid {
		print.Verb("cloned repository failed validation, cleaning up")
		os.RemoveAll(options.Dir)
		if validationErr != nil {
			return errors.Wrap(validationErr, "cloned repository is invalid")
		}
		return errors.New("cloned repository failed validation")
	}

	_, err = repo.Head()
	if err != nil {
		print.Verb("cloned repository has invalid HEAD, cleaning up")
		os.RemoveAll(options.Dir)
		return errors.Wrap(err, "cloned repository has invalid HEAD")
	}

	print.Verb("ensuring cloned package", options.Meta, "to", options.Dir)
	pcx, err := pkgcontext.NewPackageContext(pkgcontext.NewPackageContextOptions{
		GitHub:   options.GitHub,
		Auth:     options.Auth,
		Parent:   true,
		Dir:      options.Dir,
		Platform: options.Platform,
		CacheDir: options.CacheDir,
	})
	if err != nil {
		return errors.Wrap(err, "failed to read cloned repository as Pawn package")
	}

	err = pcx.EnsureDependencies(options.Context, true)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies for cloned package")
	}

	return nil
}
