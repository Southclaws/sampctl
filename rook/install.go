package rook

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Install adds a new dependency to an existing local parent package
func Install(pkg types.Package, targets []versioning.DependencyString, development bool) (err error) {
	// todo: version checks

	exists := false

	for _, target := range targets {
		for _, dep := range pkg.GetAllDependencies() {
			if dep == target {
				exists = true
			}
		}

		if !exists {
			if development {
				pkg.Development = append(pkg.Development, target)
			} else {
				pkg.Dependencies = append(pkg.Dependencies, target)
			}
		} else {
			print.Warn("target already exists in dependencies")
			return
		}
	}

	err = EnsureDependencies(&pkg)
	if err != nil {
		return
	}

	err = pkg.WriteDefinition()

	return
}

// Get simply performs a git clone of the given package to the specified directory then ensures it
func Get(meta versioning.DependencyMeta, dir string) (err error) {
	err = os.MkdirAll(dir, 0755)
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
	pkg, err := PackageFromDir(true, dir, "")
	if err != nil {
		return errors.Wrap(err, "failed to read cloned repository as Pawn package")
	}

	err = EnsureDependencies(&pkg)
	if err != nil {
		return errors.Wrap(err, "failed to ensure dependencies for cloned package")
	}

	return
}
