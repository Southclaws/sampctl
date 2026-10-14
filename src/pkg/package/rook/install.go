package rook

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitSSH "github.com/go-git/go-git/v5/plumbing/transport/ssh"
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
	if options.Context == nil {
		options.Context = context.Background()
	}

	if err := options.Context.Err(); err != nil {
		return err
	}

	err = os.MkdirAll(options.Dir, 0o700)
	if err != nil {
		return errors.Wrap(err, "failed to create directory for clone")
	}

	if !util.DirEmpty(options.Dir) {
		options.Dir = filepath.Join(options.Dir, options.Meta.Repo)
	}

	print.Verb("cloning package", options.Meta, "to", options.Dir)

	repo, err := clonePackageRepository(options)
	if err != nil {
		return errors.Wrap(err, "failed to clone package repository")
	}

	valid, validationErr := pkgcontext.ValidateRepository(options.Dir)
	if validationErr != nil || !valid {
		print.Verb("cloned repository failed validation, cleaning up")
		if removeErr := os.RemoveAll(options.Dir); removeErr != nil {
			print.Warn("failed to clean up invalid clone:", removeErr)
		}
		if validationErr != nil {
			return errors.Wrap(validationErr, "cloned repository is invalid")
		}
		return errors.New("cloned repository failed validation")
	}

	_, err = repo.Head()
	if err != nil {
		print.Verb("cloned repository has invalid HEAD, cleaning up")
		if removeErr := os.RemoveAll(options.Dir); removeErr != nil {
			print.Warn("failed to clean up clone with invalid HEAD:", removeErr)
		}
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

func clonePackageRepository(options GetOptions) (*git.Repository, error) {
	cloneOptions := &git.CloneOptions{
		URL:  options.Meta.URL(),
		Auth: packageCloneAuth(options.Meta, options.Auth),
		Tags: git.AllTags,
	}
	if options.Meta.Tag == "" && options.Meta.Commit == "" {
		cloneOptions.Depth = 1000
	}

	if options.Meta.Tag == "" && options.Meta.Commit == "" && options.Meta.Branch != "" {
		cloneOptions.ReferenceName = plumbing.NewBranchReferenceName(options.Meta.Branch)
		cloneOptions.SingleBranch = true
	}

	repo, err := git.PlainCloneContext(options.Context, options.Dir, false, cloneOptions)
	if err != nil {
		return nil, err
	}

	var ref *plumbing.Reference
	switch {
	case options.Meta.Tag == "latest":
		ref, err = latestPackageRef(repo)
	case options.Meta.Tag != "":
		ref, err = versioning.RefFromTag(repo, options.Meta)
	case options.Meta.Commit != "":
		ref, err = versioning.RefFromCommit(repo, options.Meta)
	default:
		return repo, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve requested package revision")
	}
	if ref == nil {
		return repo, nil
	}

	if err := options.Context.Err(); err != nil {
		return nil, err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: ref.Hash()}); err != nil {
		return nil, errors.Wrap(err, "failed to check out requested package revision")
	}

	return repo, nil
}

func latestPackageRef(repo *git.Repository) (*plumbing.Reference, error) {
	versions, err := versioning.GetRepoSemverTags(repo)
	if err != nil {
		return nil, err
	}
	if len(versions) > 0 {
		latest := versions[0]
		for _, version := range versions[1:] {
			if version.Version.GreaterThan(latest.Version) {
				latest = version
			}
		}

		return latest.Ref, nil
	}

	tags, err := repo.Tags()
	if err != nil {
		return nil, err
	}
	defer tags.Close()

	var latest *plumbing.Reference
	var latestTime time.Time
	err = tags.ForEach(func(tag *plumbing.Reference) error {
		ref, err := versioning.RefFromTagRef(repo, tag)
		if err != nil {
			return nil
		}
		commit, err := repo.CommitObject(ref.Hash())
		if err != nil {
			return nil
		}

		when := commit.Committer.When
		if latest == nil || when.After(latestTime) || when.Equal(latestTime) && ref.Name() > latest.Name() {
			latest, latestTime = ref, when
		}

		return nil
	})

	return latest, err
}

func packageCloneAuth(meta versioning.DependencyMeta, auth transport.AuthMethod) transport.AuthMethod {
	if bundle, ok := auth.(*pkgcontext.GitMultiAuth); ok {
		if meta.SSH != "" {
			auth = bundle.SSH
		} else {
			auth = bundle.HTTP
		}
	}

	if meta.SSH != "" {
		if auth, ok := auth.(gitSSH.AuthMethod); ok {
			return auth
		}

		return nil
	}

	if auth, ok := auth.(gitHTTP.AuthMethod); ok {
		return auth
	}

	return nil
}
