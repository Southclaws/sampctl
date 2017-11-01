package rook

import (
	"fmt"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// ensure.go contains functions to install, update and validate dependencies of a project.

// EnsureProject will load a project's json file and make sure all the necessary dependencies are
// present in the sibling directory `vendor/`.
func EnsureProject(dir string) (err error) {

	return
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func EnsurePackage(vendorDirectory string, pkg Package) (err error) {
	pkgPath := filepath.Join(vendorDirectory, pkg.repo)

	repo, err := git.PlainOpen(pkgPath)
	if err == git.ErrRepositoryNotExists {
		fmt.Println("Specified package does not exist, cloning new copy")

		repo, err = git.PlainClone(pkgPath, false, &git.CloneOptions{
			URL: pkg.GetURL(),
		})
		if err != nil {
			return
		}
	} else if err != nil {
		return
	} else {
		fmt.Println("Specified package already exists, ensuring local copy matches version constraint")
	}

	if pkg.version == "" {
		err = repo.Fetch(&git.FetchOptions{})
		if err == git.NoErrAlreadyUpToDate {
			fmt.Println("Specified package does not have version constraint and the latest copy is already present")
			return nil
		} else if err != nil {
			err = errors.Wrap(err, "failed to fetch latest package")
			return
		} else {
			fmt.Println("Specified package does not have version constraint and the latest copy has been cloned")
		}
	}

	constraint, err := semver.NewConstraint(pkg.version)
	if err != nil {
		// todo: support non-semver versioning by just using tag
		return errors.Wrap(err, "Specified package version constraint is not valid")
	}

	tags, err := repo.Tags()
	if err != nil {
		return
	}
	defer tags.Close()

	var ref *plumbing.Reference
	tags.ForEach(func(pr *plumbing.Reference) error {
		tagVersion, err := semver.NewVersion(pr.Name().Short())
		if err != nil {
			fmt.Println("tag %s is not a valid semver!", pr.Name().Short())
			return nil
		}

		if constraint.Check(tagVersion) {
			ref = pr
		}

		return nil
	})

	if ref == nil {
		err = errors.Errorf("failed to satisfy constraint '%s', no tag found by that name", constraint)
		return
	}

	wt, err := repo.Worktree()
	if err != nil {
		return
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Hash:  ref.Hash(),
		Force: true,
	})
	if err != nil {
		return
	}

	return
}

// Get will retrieve package from GitHub and place it in the specified directory.
func Get(dir string, pkg Package) (err error) {
	return
}

// CheckoutVersion will make sure a package directory (git repo) is pointing to the correct commit
// that matches the version for the dependency.
func CheckoutVersion(dir string, pkg Package) (err error) {
	return
}
