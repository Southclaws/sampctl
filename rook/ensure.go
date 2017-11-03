package rook

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/Southclaws/sampctl/util"
	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// ensure.go contains functions to install, update and validate dependencies of a project.

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func EnsurePackage(vendorDirectory string, pkg Package) (err error) {
	pkgPath := filepath.Join(util.FullPath(vendorDirectory), pkg.repo)

	repo, err := git.PlainOpen(pkgPath)
	if err != nil {
		if err == git.ErrRepositoryNotExists {
			fmt.Println(pkg, "package does not exist at", pkgPath, "cloning new copy")

			repo, err = git.PlainClone(pkgPath, false, &git.CloneOptions{
				URL: pkg.GetURL(),
			})
			if err != nil {
				err = errors.Wrap(err, "failed to clone dependency repository")
				return
			}
		} else {
			err = errors.Wrap(err, "failed to open dependency repository")
			return
		}
	} else {
		fmt.Println(pkg, "package already exists, ensuring local copy matches version constraint")
	}

	if pkg.version == "" {
		err = repo.Fetch(&git.FetchOptions{})
		if err == git.NoErrAlreadyUpToDate {
			fmt.Println(pkg, "package does not have version constraint and the latest copy is already present")
			return nil
		} else if err != nil {
			err = errors.Wrap(err, "failed to fetch latest package")
			return
		} else {
			fmt.Println(pkg, "package does not have version constraint and the latest copy has been cloned")
		}
	}

	constraint, err := semver.NewConstraint(pkg.version)
	if err != nil {
		// todo: support non-semver versioning by just using tag
		return errors.Wrap(err, "package version constraint is not valid")
	}

	tags, err := repo.Tags()
	if err != nil {
		return errors.Wrap(err, "failed to get repo tags")
	}
	defer tags.Close()

	var ref *plumbing.Reference
	versionedTags := semver.Collection{}
	allRefs := make(map[string]*plumbing.Reference)
	tags.ForEach(func(pr *plumbing.Reference) error {
		tag := pr.Name().Short()

		tagVersion, err := semver.NewVersion(tag)
		if err != nil {
			return nil
		}

		versionedTags = append(versionedTags, tagVersion)
		allRefs[tagVersion.String()] = pr

		return nil
	})

	sort.Sort(sort.Reverse(versionedTags))

	for _, version := range versionedTags {
		tag := allRefs[version.String()]

		if constraint.Check(version) {
			fmt.Println(pkg, "discovered tag", tag, "that matches constraint", pkg.version, tag.Hash().String())
			ref = tag
			break
		}

		fmt.Println(pkg, "incompatible tag", tag, "does not satisfy constraint", pkg.version, tag.Hash().String())
	}

	if ref == nil {
		err = errors.Errorf("failed to satisfy constraint, no tag found by that name, available tags: %v", versionedTags)
		return
	}

	wt, err := repo.Worktree()
	if err != nil {
		err = errors.Wrap(err, "failed to get repo worktree")
		return
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Hash:  ref.Hash(),
		Force: true,
	})
	if err != nil {
		err = errors.Wrapf(err, "failed to checkout necessary commit %s", ref.Hash())
		return
	}

	head, _ := repo.Head()
	fmt.Println(pkg, "successfully checked out to", head.Hash().String())

	return
}
