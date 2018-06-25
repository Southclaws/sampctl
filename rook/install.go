package rook

import (
	"context"
	"os"
	"path/filepath"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Install adds a new dependency to an existing local parent package
func (pcx *PackageContext) Install(ctx context.Context, targets []versioning.DependencyString, development bool) (err error) {
	exists := false

	for _, target := range targets {
		_, err = versioning.DependencyString(target).Explode()
		if err != nil {
			return errors.Wrapf(err, "failed to parse %s as a dependency string", target)
		}

		for _, dep := range pcx.Package.GetAllDependencies() {
			if dep == target {
				exists = true
			}
		}

		if !exists {
			if development {
				pcx.Package.Development = append(pcx.Package.Development, target)
			} else {
				pcx.Package.Dependencies = append(pcx.Package.Dependencies, target)
			}
		} else {
			print.Warn("target already exists in dependencies")
			return
		}
	}

	print.Verb(pcx.Package, "ensuring dependencies are cached for package context")
	err = pcx.EnsureDependenciesCached()
	if err != nil {
		return
	}

	print.Verb(pcx.Package, "ensuring dependencies are installed for package context")
	err = pcx.EnsureDependencies(ctx, true)
	if err != nil {
		return
	}

	err = pcx.Package.WriteDefinition()

	return
}

// Get simply performs a git clone of the given package to the specified directory then ensures it
func Get(ctx context.Context, gh *github.Client, meta versioning.DependencyMeta, dir string, auth transport.AuthMethod, platform, cacheDir string) (err error) {
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
	pcx, err := NewPackageContext(gh, auth, true, dir, platform, cacheDir, "")
	if err != nil {
		return errors.Wrap(err, "failed to read cloned repository as Pawn package")
	}

	err = pcx.EnsureDependencies(ctx, true)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies for cloned package")
	}

	return
}
